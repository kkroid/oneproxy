#include "../instance_guard.h"
#include <QCoreApplication>
#include <QTimer>
#include <cstdio>
#ifdef Q_OS_WIN
#include <qt_windows.h>
#endif

int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    if (argc != 3) return 3;
    InstanceGuard guard(QString::fromLocal8Bit(argv[2]));
    if (QString::fromLocal8Bit(argv[1]) == "--legacy") {
#ifdef Q_OS_WIN
        WNDCLASSW wc = {};
        wc.lpfnWndProc = DefWindowProcW;
        wc.hInstance = GetModuleHandleW(nullptr);
        wc.lpszClassName = L"OneProxyTrayWindow";
        if (!RegisterClassW(&wc) || !CreateWindowExW(0, wc.lpszClassName, L"", 0,
                0, 0, 0, 0, HWND_MESSAGE, nullptr, wc.hInstance, nullptr)) return 4;
#else
        return 4;
#endif
    } else {
        const auto error = guard.acquire();
        if (!error.isEmpty()) {
            std::fprintf(stderr, "%s\n", error.toUtf8().constData());
            return 2;
        }
    }
    std::puts("READY");
    std::fflush(stdout);
    // Bound fixture lifetime even if a test runner is interrupted.
    QTimer::singleShot(30000, &app, &QCoreApplication::quit);
    return app.exec();
}
