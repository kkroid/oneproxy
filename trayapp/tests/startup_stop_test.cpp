// Appended to a build-directory copy of main.cpp by CMake so this regression
// invokes the actual tray methods. Only access control and main's name change.
#include <condition_variable>
#include <mutex>
#include <chrono>
#include <cstdlib>

namespace {
std::mutex dllMutex;
std::condition_variable changed;
unsigned long long dllEpoch = 0;
bool entered = false;
bool stopped = false;
bool downloading = false;
bool pauseBeforeDLL = false;
int registered = 0;
int saved = 0;
int connected = 0;
int stopCalls = 0;
int epochCalls = 0;

unsigned long long fakeEpoch() {
    std::lock_guard<std::mutex> lock(dllMutex);
    ++epochCalls;
    return dllEpoch;
}

char* fakeStart(char*, unsigned long long epoch) {
    std::unique_lock<std::mutex> lock(dllMutex);
    entered = true;
    changed.notify_all();
    // Force the reviewer-reported interleaving: Stop wins before DLL admission.
    if (pauseBeforeDLL) changed.wait(lock, [] { return stopped; });
    if (epoch != dllEpoch) return nullptr;
    ++registered;
    downloading = true;
    changed.notify_all();
    changed.wait(lock, [] { return stopped; });
    // Model the existing DLL's session check after a late download completes.
    if (epoch != dllEpoch) return nullptr;
    ++saved;
    ++connected;
    return nullptr;
}

char* fakeStop() {
    std::lock_guard<std::mutex> lock(dllMutex);
    ++stopCalls;
    ++dllEpoch;
    stopped = true;
    changed.notify_all();
    return nullptr;
}
char* fakeStatus() { return nullptr; }

bool exercise(bool beforeDLL) {
    entered = stopped = downloading = false;
    registered = saved = connected = stopCalls = epochCalls = 0;
    pauseBeforeDLL = beforeDLL;
    OneProxyTray tray;
    tray.timer.stop();
    tray.doStart();
    {
        std::unique_lock<std::mutex> lock(dllMutex);
        if (!changed.wait_for(lock, std::chrono::seconds(2), [beforeDLL] {
                return beforeDLL ? entered : downloading;
            })) {
            qCritical() << "worker did not reach the requested boundary";
            std::abort(); // Bound failures without destroying a joinable thread.
        }
    }
    tray.doStop(); // Calls the real stopStartupWorker and joins the late worker.
    QCoreApplication::processEvents(); // Deliver the old completion callback.
    const bool passed = epochCalls == 1 && stopCalls == 1 && saved == 0 &&
                        connected == 0 && registered == (beforeDLL ? 0 : 1) &&
                        !tray.starting && !tray.startupThread.joinable();
    qInfo() << (beforeDLL ? "before DLL" : "during download")
            << "registered=" << registered << "saved=" << saved
            << "connected=" << connected << "stop_calls=" << stopCalls;
    tray.tray->hide();
    delete tray.menu;
    return passed;
}
}

int main(int argc, char** argv) {
    QApplication app(argc, argv);
    pStartupEpoch = fakeEpoch;
    pStartAtEpoch = fakeStart;
    pStop = fakeStop;
    pStatus = fakeStatus;
    return exercise(true) && exercise(false) ? 0 : 1;
}
