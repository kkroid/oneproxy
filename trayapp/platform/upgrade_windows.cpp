// Loaded from Setup's private temporary directory, never from the installation.
// No Qt/Go runtime dependency. Setup retains these handles until it finishes.
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <tlhelp32.h>
#include <string>
#include <vector>
#include <algorithm>

#ifdef _M_IX86
#pragma comment(linker, "/EXPORT:PrepareUpgrade=_PrepareUpgrade@4")
#pragma comment(linker, "/EXPORT:FinishUpgrade=_FinishUpgrade@0")
#pragma comment(linker, "/EXPORT:UpgradeError=_UpgradeError@8")
#endif

namespace {
std::vector<HANDLE> guards;
std::wstring failure;
struct Process {
    DWORD pid;
    HANDLE handle;
    FILETIME created;
    bool cooperative = false;
    bool consoleHost = false;
};
bool fail(const std::wstring &operation, DWORD error = GetLastError())
{
    failure = operation + L" (Windows error " + std::to_wstring(error) + L").";
    return false;
}
bool equal(const std::wstring &a, const std::wstring &b)
{
    return _wcsicmp(a.c_str(), b.c_str()) == 0;
}
std::wstring canonical(const std::wstring &path)
{
#ifndef _WIN64
    // QueryFullProcessImageName returns the native image path. An x86 Setup
    // must not redirect System32 to SysWOW64 when inspecting a 64-bit child.
    PVOID redirection = nullptr;
    BOOL disabled = Wow64DisableWow64FsRedirection(&redirection);
#endif
    HANDLE file = CreateFileW(path.c_str(), 0, FILE_SHARE_READ | FILE_SHARE_WRITE |
        FILE_SHARE_DELETE, nullptr, OPEN_EXISTING, FILE_FLAG_BACKUP_SEMANTICS, nullptr);
#ifndef _WIN64
    DWORD error = GetLastError();
    if (disabled) Wow64RevertWow64FsRedirection(redirection);
    SetLastError(error);
#endif
    if (file == INVALID_HANDLE_VALUE) return {};
    wchar_t result[32768];
    DWORD length = GetFinalPathNameByHandleW(file, result, 32768, FILE_NAME_NORMALIZED);
    CloseHandle(file);
    return length && length < 32768 ? std::wstring(result, length) : std::wstring();
}
std::wstring imagePath(HANDLE process)
{
    wchar_t path[32768];
    DWORD length = 32768;
    if (!QueryFullProcessImageNameW(process, 0, path, &length)) return {};
    return canonical(std::wstring(path, length));
}
bool alive(HANDLE process) { return WaitForSingleObject(process, 0) == WAIT_TIMEOUT; }
void closeProcesses(std::vector<Process> &processes)
{
    for (auto &p : processes) CloseHandle(p.handle);
    processes.clear();
}
bool discover(const std::wstring &root, std::vector<Process> &parents,
              std::vector<Process> &children, bool childrenOnly)
{
    HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (snapshot == INVALID_HANDLE_VALUE) return fail(L"Cannot enumerate processes");
    PROCESSENTRY32W entry = {sizeof(entry)};
    bool ok = true;
    for (BOOL more = Process32FirstW(snapshot, &entry); more; more = Process32NextW(snapshot, &entry)) {
        const auto parentIterator = std::find_if(parents.begin(), parents.end(), [&](const Process &p) {
            return p.pid == entry.th32ParentProcessID;
        });
        const auto childIterator = std::find_if(children.begin(), children.end(), [&](const Process &p) {
            return p.pid == entry.th32ParentProcessID;
        });
        const Process *parent = parentIterator != parents.end() ? &*parentIterator :
            (childIterator != children.end() ? &*childIterator : nullptr);
        const bool child = parent != nullptr;
        const bool candidate = equal(entry.szExeFile, L"oneproxy-tray.exe") ||
                               equal(entry.szExeFile, L"oneproxy.exe");
        if ((!candidate || childrenOnly) && !child) continue;
        HANDLE handle = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE |
                                    PROCESS_TERMINATE, FALSE, entry.th32ProcessID);
        if (!handle) {
            if (GetLastError() == ERROR_INVALID_PARAMETER) continue; // exited during snapshot
            ok = fail(L"Cannot verify process " + std::to_wstring(entry.th32ProcessID));
            break;
        }
        if (!alive(handle)) { CloseHandle(handle); continue; }
        FILETIME created, exited, kernel, user;
        const auto path = imagePath(handle);
        if (path.empty() || !GetProcessTimes(handle, &created, &exited, &kernel, &user)) {
            if (!alive(handle)) { CloseHandle(handle); continue; }
            ok = fail(L"Cannot verify process identity: " + std::wstring(entry.szExeFile) +
                      L" PID " + std::to_wstring(entry.th32ProcessID)); CloseHandle(handle); break;
        }
        const bool ours = equal(path, root + L"\\oneproxy-tray.exe") ||
                          equal(path, root + L"\\oneproxy.exe");
        if (child && CompareFileTime(&created, &parent->created) >= 0) {
            wchar_t systemDirectory[MAX_PATH];
            GetSystemDirectoryW(systemDirectory, MAX_PATH);
            const bool console = equal(path, canonical(std::wstring(systemDirectory) + L"\\conhost.exe"));
            if (!equal(path, root + L"\\bin\\sing-box.exe") && !console) {
                ok = fail(L"Unrecognized child of target instance; refusing to stop it: " + path, ERROR_INVALID_DATA);
                CloseHandle(handle); break;
            }
            auto found = std::find_if(children.begin(), children.end(), [&](const Process &p) {
                return p.pid == entry.th32ProcessID;
            });
            if (found == children.end()) children.push_back({entry.th32ProcessID, handle, created, false, console});
            else CloseHandle(handle);
        } else if (ours && !childrenOnly) {
            parents.push_back({entry.th32ProcessID, handle, created});
        } else CloseHandle(handle);
    }
    if (ok && GetLastError() != ERROR_NO_MORE_FILES) ok = fail(L"Process enumeration was interrupted");
    CloseHandle(snapshot);
    return ok;
}
bool stop(const std::wstring &root)
{
    std::vector<Process> parents, children;
    bool ok = discover(root, parents, children, false);
    // Repeat until stable so children preceding their ancestors are not missed.
    auto discoverChildren = [&]() {
        size_t previous;
        do {
            previous = children.size();
            if (!discover(root, parents, children, true)) return false;
        } while (previous != children.size());
        return true;
    };
    if (ok) ok = discoverChildren();
    if (ok) {
        HWND window = nullptr;
        while ((window = FindWindowExW(HWND_MESSAGE, window, L"OneProxyTrayWindow", nullptr))) {
            DWORD pid = 0;
            DWORD thread = GetWindowThreadProcessId(window, &pid);
            for (auto &p : parents) if (p.pid == pid && alive(p.handle)) {
                p.cooperative = GetPropW(window, L"OneProxy.UpgradeQuit.v1") != nullptr;
                // Current builds explicitly acknowledge this message. Older Qt builds
                // receive WM_QUIT on their actual GUI thread, not WM_CLOSE on a helper HWND.
                DWORD_PTR acknowledged = 0;
                if (!SendMessageTimeoutW(window, RegisterWindowMessageW(L"OneProxy.UpgradeQuit.v1"),
                    0, 0, SMTO_ABORTIFHUNG | SMTO_BLOCK, 1000, &acknowledged) || acknowledged != 1)
                    PostThreadMessageW(thread, WM_QUIT, 0, 0);
            }
        }
        for (auto &p : parents) {
            if (WaitForSingleObject(p.handle, 5000) == WAIT_TIMEOUT) {
                if (p.cooperative) {
                    ok = fail(L"OneProxy did not finish its requested shutdown; retry or cancel", WAIT_TIMEOUT);
                    break;
                }
                // The held handle identifies the exact verified process, never a reused PID.
                if (!TerminateProcess(p.handle, ERROR_INSTALL_SUSPEND) && alive(p.handle)) {
                    ok = fail(L"Cannot stop target instance"); break;
                }
            }
            if (WaitForSingleObject(p.handle, 5000) != WAIT_OBJECT_0) {
                ok = fail(L"Target instance did not exit", WAIT_TIMEOUT); break;
            }
        }
        // Catch a child created during graceful shutdown. Parent handles remain held.
        if (ok) ok = discoverChildren();
        std::stable_sort(children.begin(), children.end(), [](const Process &a, const Process &b) {
            return a.consoleHost < b.consoleHost;
        });
        if (ok) for (auto &p : children) {
            // Windows owns console hosts. Observe their natural exit, never kill them.
            if (p.consoleHost) {
                if (WaitForSingleObject(p.handle, 5000) != WAIT_OBJECT_0) {
                    ok = fail(L"Target console host did not exit", WAIT_TIMEOUT); break;
                }
                continue;
            }
            if (WaitForSingleObject(p.handle, 2000) == WAIT_TIMEOUT &&
                !TerminateProcess(p.handle, ERROR_INSTALL_SUSPEND) && alive(p.handle)) {
                ok = fail(L"Cannot stop verified target child"); break;
            }
            if (WaitForSingleObject(p.handle, 5000) != WAIT_OBJECT_0) {
                ok = fail(L"Target child did not exit", WAIT_TIMEOUT); break;
            }
        }
    }
    closeProcesses(children); closeProcesses(parents);
    return ok;
}
bool guardBinaries(const std::wstring &directory)
{
    WIN32_FIND_DATAW entry;
    HANDLE search = FindFirstFileW((directory + L"\\*").c_str(), &entry);
    if (search == INVALID_HANDLE_VALUE) return fail(L"Cannot inspect installation files");
    bool ok = true;
    do {
        if (equal(entry.cFileName, L".") || equal(entry.cFileName, L"..")) continue;
        const auto path = directory + L"\\" + entry.cFileName;
        if (entry.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT) {
            ok = fail(L"Installation contains a reparse point: " + path, ERROR_NOT_SUPPORTED); break;
        }
        if (entry.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY) {
            if (!(ok = guardBinaries(path))) break;
        } else {
            const auto dot = path.find_last_of(L'.');
            if (dot == std::wstring::npos ||
                (!equal(path.substr(dot), L".exe") && !equal(path.substr(dot), L".dll") &&
                 !equal(path.substr(dot), L".srs") && !equal(path.substr(dot), L".ico") &&
                 !equal(entry.cFileName, L"config-placeholder.json"))) continue;
            // Opening for write fails for an existing executable image mapping.
            // Denying read prevents a legacy executable/DLL from loading in the
            // scan-to-copy gap, while allowing Setup to replace the file.
            HANDLE file = CreateFileW(path.c_str(), GENERIC_WRITE, FILE_SHARE_WRITE | FILE_SHARE_DELETE,
                                      nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
            if (file == INVALID_HANDLE_VALUE) {
                ok = fail(L"File is in use or not writable: " + path); break;
            }
            guards.push_back(file);
        }
    } while (FindNextFileW(search, &entry));
    if (ok && GetLastError() != ERROR_NO_MORE_FILES) ok = fail(L"File enumeration was interrupted");
    FindClose(search);
    return ok;
}
}

extern "C" __declspec(dllexport) void __stdcall FinishUpgrade()
{
    for (auto handle : guards) CloseHandle(handle);
    guards.clear();
}
extern "C" __declspec(dllexport) void __stdcall UpgradeError(wchar_t *buffer, int count)
{
    if (count > 0) wcsncpy_s(buffer, count, failure.c_str(), _TRUNCATE);
}
extern "C" __declspec(dllexport) int __stdcall PrepareUpgrade(const wchar_t *directory)
{
    FinishUpgrade();
    failure.clear();
    const auto root = canonical(directory);
    if (root.empty()) return fail(L"Cannot resolve installation directory"), 0;
    HANDLE marker = CreateFileW((root + L"\\.oneproxy-upgrading").c_str(), GENERIC_WRITE | DELETE,
        FILE_SHARE_READ, nullptr, CREATE_ALWAYS, FILE_ATTRIBUTE_HIDDEN | FILE_FLAG_DELETE_ON_CLOSE, nullptr);
    if (marker == INVALID_HANDLE_VALUE) return fail(L"Another upgrade is running, or directory is not writable"), 0;
    guards.push_back(marker);
    if (!stop(root) || !guardBinaries(root)) { FinishUpgrade(); return 0; }
    return 1;
}
