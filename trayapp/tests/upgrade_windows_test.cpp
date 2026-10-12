// Native integration harness: each run gets a private desktop, profile, and Job.
// The helper is linked from the same source used by the installer DLL.
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <tlhelp32.h>
#include <filesystem>
#include <algorithm>
#include <fstream>
#include <iostream>
#include <stdexcept>
#include <string>

extern "C" int __stdcall PrepareUpgrade(const wchar_t *);
extern "C" void __stdcall FinishUpgrade();
extern "C" void __stdcall ReleaseUninstallGuards();
extern "C" void __stdcall UpgradeError(wchar_t *, int);
namespace fs = std::filesystem;
HANDLE job;
std::wstring desktop;
fs::path profileShim;
void require(bool condition, const char *message)
{
    if (!condition) throw std::runtime_error(std::string(message) + " error=" + std::to_string(GetLastError()));
    std::cout << "PASS " << message << std::endl;
}
PROCESS_INFORMATION spawn(const fs::path &file, const std::wstring &args, const fs::path &profile,
                          bool check = true)
{
    fs::create_directories(profile);
    SetEnvironmentVariableW(L"USERPROFILE", profile.c_str());
    SetEnvironmentVariableW(L"HOMEDRIVE", profile.root_name().c_str());
    SetEnvironmentVariableW(L"HOMEPATH", profile.relative_path().wstring().insert(0, L"\\").c_str());
    auto command = L"\"" + file.wstring() + L"\" " + args;
    STARTUPINFOW startup = {sizeof(startup)};
    startup.lpDesktop = desktop.data();
    PROCESS_INFORMATION process = {};
    BOOL started = CreateProcessW(file.c_str(), command.data(), nullptr, nullptr, FALSE,
        CREATE_SUSPENDED | CREATE_NO_WINDOW, nullptr, file.parent_path().c_str(), &startup, &process);
    if (check) require(started, "isolated process created");
    if (started) {
        std::wcout << L"Fixture PID=" << process.dwProcessId << L" path=" << file.wstring() << std::endl;
        require(AssignProcessToJobObject(job, process.hProcess), "fixture cleanup job assigned");
        if (file.filename() == L"oneproxy-tray.exe" && fs::exists(file.parent_path() / L"Qt6Core.dll")) {
            auto bytes = (profileShim.wstring().size() + 1) * sizeof(wchar_t);
            void *remote = VirtualAllocEx(process.hProcess, nullptr, bytes, MEM_COMMIT | MEM_RESERVE, PAGE_READWRITE);
            require(remote && WriteProcessMemory(process.hProcess, remote, profileShim.c_str(), bytes, nullptr),
                    "test-only Qt profile shim staged");
            auto loader = reinterpret_cast<LPTHREAD_START_ROUTINE>(GetProcAddress(GetModuleHandleW(L"kernel32.dll"), "LoadLibraryW"));
            HANDLE thread = CreateRemoteThread(process.hProcess, nullptr, 0, loader, remote, 0, nullptr);
            require(thread && WaitForSingleObject(thread, 10000) == WAIT_OBJECT_0, "profile shim loaded before tray entry");
            DWORD loaded = 0; GetExitCodeThread(thread, &loaded); CloseHandle(thread);
            require(loaded != 0, "Qt profile import redirected (no release-file changes)");
            VirtualFreeEx(process.hProcess, remote, 0, MEM_RELEASE);
        }
        ResumeThread(process.hThread); CloseHandle(process.hThread);
    }
    return process;
}
DWORD childOf(DWORD parent)
{
    HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    PROCESSENTRY32W entry = {sizeof(entry)};
    DWORD child = 0;
    for (BOOL more = Process32FirstW(snapshot, &entry); more; more = Process32NextW(snapshot, &entry))
        if (entry.th32ParentProcessID == parent && _wcsicmp(entry.szExeFile, L"sing-box.exe") == 0)
            child = entry.th32ProcessID;
    CloseHandle(snapshot); return child;
}
HANDLE waitChild(DWORD parent)
{
    DWORD child = 0;
    for (int i = 0; i < 200 && !child; ++i) { child = childOf(parent); Sleep(50); }
    if (!child) {
        HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, parent);
        DWORD code = 0; GetExitCodeProcess(process, &code); CloseHandle(process);
        std::cout << "Parent exit code=" << code << std::endl;
        EnumWindows([](HWND window, LPARAM) -> BOOL {
            wchar_t text[2048]; GetWindowTextW(window, text, 2048); std::wcerr << text << std::endl;
            EnumChildWindows(window, [](HWND childWindow, LPARAM) -> BOOL {
                wchar_t text[2048]; GetWindowTextW(childWindow, text, 2048); std::wcerr << text << std::endl;
                return TRUE;
            }, 0);
            return TRUE;
        }, 0);
    }
    require(child != 0, "managed sing-box fixture started by real core");
    HANDLE handle = OpenProcess(SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION, FALSE, child);
    require(handle != nullptr, "child identity retained");
    return handle;
}
DWORD trayThread(DWORD pid)
{
    for (int i = 0; i < 100; ++i) {
        HWND window = nullptr;
        while ((window = FindWindowExW(HWND_MESSAGE, window, L"OneProxyTrayWindow", nullptr))) {
            DWORD owner;
            DWORD thread = GetWindowThreadProcessId(window, &owner);
            if (owner == pid) return thread;
        }
        Sleep(50);
    }
    return 0;
}
void prepared(const fs::path &directory)
{
    if (!PrepareUpgrade(directory.c_str())) {
        wchar_t error[2048]; UpgradeError(error, 2048);
        std::wcerr << error << std::endl;
        require(false, "upgrade prepared");
    }
    require(true, "upgrade prepared");
}
HWND button(const wchar_t *caption)
{
    struct Search { const wchar_t *caption; HWND found; } search{caption, nullptr};
    EnumWindows([](HWND window, LPARAM value) -> BOOL {
        EnumChildWindows(window, [](HWND child, LPARAM value) -> BOOL {
            auto &search = *reinterpret_cast<Search *>(value);
            wchar_t text[128]; GetWindowTextW(child, text, 128);
            if (IsWindowVisible(child) && IsWindowEnabled(child) && !wcscmp(text, search.caption))
                search.found = child;
            return TRUE;
        }, value);
        return TRUE;
    }, reinterpret_cast<LPARAM>(&search));
    return search.found;
}
void click(HWND window) { PostMessageW(window, BM_CLICK, 0, 0); }
HWND dialogButton(int id)
{
    struct Search { int id; HWND found; } search{id, nullptr};
    EnumWindows([](HWND window, LPARAM value) -> BOOL {
        auto &search = *reinterpret_cast<Search *>(value);
        wchar_t name[32]; GetClassNameW(window, name, 32);
        if (IsWindowVisible(window) && !wcscmp(name, L"#32770")) {
            HWND button = GetDlgItem(window, search.id);
            if (button && IsWindowEnabled(button)) search.found = button;
        }
        return TRUE;
    }, reinterpret_cast<LPARAM>(&search));
    return search.found;
}
int wmain(int argc, wchar_t **argv)
{
    if (argc > 1 && (std::wstring(argv[1]) == L"--child" || std::wstring(argv[1]) == L"run")) {
        Sleep(INFINITE); return 0;
    }
    if (argc > 1 && (std::wstring(argv[1]) == L"--fixture" || std::wstring(argv[1]) == L"--hung")) {
        wchar_t file[32768]; GetModuleFileNameW(nullptr, file, 32768);
        auto child = fs::path(file).parent_path() / L"bin/sing-box.exe";
        auto command = L"\"" + child.wstring() + L"\" --child";
        STARTUPINFOW startup = {sizeof(startup)}; PROCESS_INFORMATION process = {};
        if (!CreateProcessW(child.c_str(), command.data(), nullptr, nullptr, FALSE, CREATE_NO_WINDOW,
                            nullptr, nullptr, &startup, &process)) return 2;
        CloseHandle(process.hThread); CloseHandle(process.hProcess);
        if (std::wstring(argv[1]) == L"--hung") {
            WNDCLASSW wc = {};
            wc.lpfnWndProc = [](HWND w, UINT m, WPARAM wp, LPARAM lp) -> LRESULT {
                if (m == RegisterWindowMessageW(L"OneProxy.UpgradeQuit.v1")) return 1;
                return DefWindowProcW(w, m, wp, lp);
            };
            wc.hInstance = GetModuleHandleW(nullptr); wc.lpszClassName = L"OneProxyTrayWindow";
            RegisterClassW(&wc);
            HWND window = CreateWindowW(wc.lpszClassName, L"", 0, 0, 0, 0, 0, HWND_MESSAGE, nullptr, wc.hInstance, nullptr);
            SetPropW(window, L"OneProxy.UpgradeQuit.v1", reinterpret_cast<HANDLE>(1));
            MSG message;
            while (GetMessageW(&message, nullptr, 0, 0)) DispatchMessageW(&message);
        }
        Sleep(INFINITE); return 0;
    }
    if (argc < 3) return 2;
    try {
        fs::path root = fs::absolute(argv[2]);
        profileShim = fs::absolute(argv[0]).parent_path() / L"upgrade-profile-shim.dll";
        desktop = L"OneProxyUpgradeTest-" + std::to_wstring(GetCurrentProcessId());
        HDESK isolated = CreateDesktopW(desktop.c_str(), nullptr, nullptr, 0, GENERIC_ALL, nullptr);
        require(isolated && SetThreadDesktop(isolated), "private desktop active");
        job = CreateJobObjectW(nullptr, nullptr);
        JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits = {};
        limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
        require(SetInformationJobObject(job, JobObjectExtendedLimitInformation, &limits, sizeof(limits)),
                "cleanup job configured");
        const std::wstring mode = argv[1];
        const bool setupMode = mode == L"--setup" || mode == L"--setup-locked";
        if (mode == L"--handle-test") {
            fs::create_directories(root);
            const auto unins = root / L"unins000.exe";
            const auto replacement = root / L"replacement.tmp";
            std::ofstream(unins) << "original";
            std::ofstream(replacement) << "replacement";
            std::ofstream(root / L"unins000.dat") << "uninstall log";
            std::ofstream(root / L"test.dll") << "application DLL";
            for (DWORD sharing : {DWORD(FILE_SHARE_WRITE | FILE_SHARE_DELETE),
                                  DWORD(FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE)}) {
                HANDLE file = CreateFileW(unins.c_str(), GENERIC_WRITE, sharing,
                    nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
                require(file != INVALID_HANDLE_VALUE, "baseline-style uninstall guard acquired");
                const BOOL moved = MoveFileExW(replacement.c_str(), unins.c_str(), MOVEFILE_REPLACE_EXISTING);
                const DWORD error = GetLastError();
                std::cout << "MoveFileEx with GENERIC_WRITE sharing=" << sharing << " error=" << error << std::endl;
                require(!moved && error == ERROR_ACCESS_DENIED, "held write handle reproduces MoveFileEx code 5 without another process");
                CloseHandle(file);
            }
            require(MoveFileExW(replacement.c_str(), unins.c_str(), MOVEFILE_REPLACE_EXISTING),
                    "closing the responsible handle restores replacement");
            prepared(root);
            ReleaseUninstallGuards();
            HANDLE uninstaller = CreateFileW(unins.c_str(), GENERIC_READ | GENERIC_WRITE | DELETE,
                0, nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
            require(uninstaller != INVALID_HANDLE_VALUE, "Inno files handed back before installation");
            CloseHandle(uninstaller);
            HANDLE dll = CreateFileW((root / L"test.dll").c_str(), GENERIC_READ,
                FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, nullptr, OPEN_EXISTING, 0, nullptr);
            require(dll == INVALID_HANDLE_VALUE, "application DLL guard survives uninstall handoff");
            require(fs::exists(root / L".oneproxy-upgrading"), "upgrade marker survives uninstall handoff");
            FinishUpgrade();
        } else if (mode.find(L"--uninstall-") == 0) {
            require(argc == 4, "isolated setup supplied");
            const auto file = root / (mode == L"--uninstall-dat" ? L"unins000.dat" : L"unins000.exe");
            HANDLE locked = INVALID_HANDLE_VALUE;
            PROCESS_INFORMATION uninstaller = {};
            if (mode == L"--uninstall-running") {
                uninstaller = spawn(file, L"", root / L"profile");
                Sleep(2000); // Let Inno's temporary second phase reach its confirmation dialog.
                require(WaitForSingleObject(uninstaller.hProcess, 0) == WAIT_TIMEOUT,
                        "real uninstaller remains active before confirmation");
            } else if (mode == L"--uninstall-readonly") {
                require(SetFileAttributesW(file.c_str(), FILE_ATTRIBUTE_READONLY), "read-only uninstall fixture set");
            } else if (mode != L"--uninstall-permission" && mode != L"--uninstall-late") {
                locked = CreateFileW(file.c_str(), GENERIC_READ, FILE_SHARE_READ,
                    nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
                require(locked != INVALID_HANDLE_VALUE, "external uninstall file lock held");
            }
            const bool interactive = mode == L"--uninstall-retry" || mode == L"--uninstall-cancel";
            auto setup = spawn(fs::absolute(argv[3]), (interactive ? L"" : std::wstring(L"/VERYSILENT /SUPPRESSMSGBOXES ")) + L"/NORESTART /SP- /DIR=\"" +
                root.wstring() + L"\" /LOG=\"" + (root / (mode + L".log")).wstring() + L"\"", root / L"profile");
            bool retried = false;
            if (mode == L"--uninstall-late") {
                const auto deadline = GetTickCount64() + 30000;
                while (GetTickCount64() < deadline && WaitForSingleObject(setup.hProcess, 0) == WAIT_TIMEOUT) {
                    std::ifstream stream(root / (mode + L".log"), std::ios::binary);
                    std::string log((std::istreambuf_iterator<char>(stream)), {});
                    log.erase(std::remove(log.begin(), log.end(), '\0'), log.end());
                    if (log.find("Starting the installation process.") != std::string::npos) {
                        locked = CreateFileW(file.c_str(), GENERIC_READ, FILE_SHARE_READ,
                            nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
                        if (locked != INVALID_HANDLE_VALUE) break;
                    }
                    Sleep(1);
                }
                require(locked != INVALID_HANDLE_VALUE, "late external EXE lock acquired after preparation handoff");
            }
            if (interactive) {
                const auto deadline = GetTickCount64() + 90000;
                while (WaitForSingleObject(setup.hProcess, 100) == WAIT_TIMEOUT && GetTickCount64() < deadline) {
                    if (HWND yes = dialogButton(IDYES)) click(yes);
                    else if (HWND retry = dialogButton(IDRETRY)) {
                        // The posted click can take more than one poll to dismiss
                        // the modal dialog. Never click a second Retry while it closes.
                        if (!retried) {
                            require(true, "preparation failure reached Retry dialog");
                            retried = true;
                            if (mode == L"--uninstall-retry") {
                                CloseHandle(locked); locked = INVALID_HANDLE_VALUE;
                                click(retry);
                            } else click(dialogButton(IDCANCEL));
                        }
                    } else if (retried && mode == L"--uninstall-cancel") {
                        click(button(L"Cancel"));
                    } else if (HWND next = button(L"&Next >")) click(next);
                    else if (HWND next = button(L"&Next")) click(next);
                    else if (HWND install = button(L"&Install")) click(install);
                    else if (HWND finish = button(L"&Finish")) click(finish);
                    else if (HWND yes = button(L"&Yes")) click(yes);
                    Sleep(200);
                }
                require(retried, "interactive preparation Retry/Cancel page exercised");
            }
            require(WaitForSingleObject(setup.hProcess, interactive ? 0 : 120000) == WAIT_OBJECT_0, "locked uninstall setup completed");
            DWORD code; GetExitCodeProcess(setup.hProcess, &code);
            std::cout << "Setup exit=" << code << std::endl;
            require(code == (mode == L"--uninstall-retry" ? 0 : mode == L"--uninstall-late" ? 5 : 7),
                    "uninstall conflict reports expected preparation failure or retry success");
            CloseHandle(setup.hProcess);
            if (locked != INVALID_HANDLE_VALUE) CloseHandle(locked);
            if (mode == L"--uninstall-readonly") SetFileAttributesW(file.c_str(), FILE_ATTRIBUTE_NORMAL);
            if (uninstaller.hProcess) {
                require(WaitForSingleObject(uninstaller.hProcess, 0) == WAIT_TIMEOUT,
                        "upgrade did not terminate active uninstall transaction");
                CloseHandle(uninstaller.hProcess);
            }
        } else if (mode == L"--old" || mode == L"--new" || setupMode) {
            auto process = spawn(root / L"oneproxy-tray.exe", L"", root / L"profile");
            HANDLE child = waitChild(process.dwProcessId);
            require(trayThread(process.dwProcessId) != 0, "real Qt message-only window discovered");
            if (mode == L"--old") {
                require(PostThreadMessageW(trayThread(process.dwProcessId), WM_QUIT, 0, 0), "legacy WM_QUIT posted");
                require(WaitForSingleObject(process.hProcess, 10000) == WAIT_OBJECT_0, "legacy Qt exits without forced termination");
                require(WaitForSingleObject(child, 5000) == WAIT_OBJECT_0, "legacy Job child exits with Qt");
                CloseHandle(child); CloseHandle(process.hProcess);
                process = spawn(root / L"oneproxy-tray.exe", L"", root / L"profile");
                child = waitChild(process.dwProcessId);
            }
            if (setupMode) {
                require(argc == 4, "isolated setup supplied");
                HANDLE locked = INVALID_HANDLE_VALUE;
                if (mode == L"--setup-locked") {
                    locked = CreateFileW((root / L"Qt6Svg.dll").c_str(), GENERIC_READ, FILE_SHARE_READ,
                        nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
                    require(locked != INVALID_HANDLE_VALUE, "setup payload lock held");
                }
                auto setup = spawn(fs::absolute(argv[3]), L"/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /DIR=\"" +
                    root.wstring() + L"\" /LOG=\"" + (root / (mode + L".log")).wstring() + L"\"", root / L"profile");
                require(WaitForSingleObject(setup.hProcess, 120000) == WAIT_OBJECT_0, "isolated full setup completed");
                DWORD code; GetExitCodeProcess(setup.hProcess, &code);
                std::cout << "Setup exit=" << code << std::endl;
                require(code == (mode == L"--setup-locked" ? 7 : 0), "setup reports expected success/preparation failure");
                if (locked != INVALID_HANDLE_VALUE) CloseHandle(locked);
                CloseHandle(setup.hProcess);
            } else prepared(root);
            require(WaitForSingleObject(process.hProcess, 0) == WAIT_OBJECT_0, "target tray confirmed exited");
            DWORD exitCode;
            GetExitCodeProcess(process.hProcess, &exitCode);
            require(exitCode != ERROR_INSTALL_SUSPEND, "real Qt tray used graceful shutdown, not force fallback");
            require(WaitForSingleObject(child, 0) == WAIT_OBJECT_0, "target child confirmed exited");
            if (!setupMode) {
                auto blocked = spawn(root / L"oneproxy-tray.exe", L"", root / L"profile", false);
                require(!blocked.hProcess, "legacy relaunch blocked throughout copy phase");
                FinishUpgrade();
                require(!fs::exists(root / L".oneproxy-upgrading"), "upgrade marker released");
                if (mode == L"--new") {
                    HANDLE marker = CreateFileW((root / L".oneproxy-upgrading").c_str(), GENERIC_WRITE | DELETE,
                        FILE_SHARE_READ, nullptr, CREATE_ALWAYS, FILE_FLAG_DELETE_ON_CLOSE, nullptr);
                    require(marker != INVALID_HANDLE_VALUE, "copy-phase marker fixture acquired");
                    for (auto executable : {L"oneproxy-tray.exe", L"oneproxy.exe"}) {
                        auto contender = spawn(root / executable, L"--self-check", root / L"profile");
                        require(WaitForSingleObject(contender.hProcess, 10000) == WAIT_OBJECT_0,
                                "new contender exits while marker held");
                        DWORD code; GetExitCodeProcess(contender.hProcess, &code);
                        require(code == 1 && childOf(contender.dwProcessId) == 0, "contender cannot start core during upgrade");
                        CloseHandle(contender.hProcess);
                    }
                    CloseHandle(marker);
                }
            }
            CloseHandle(child); CloseHandle(process.hProcess);
        } else if (mode == L"--fixture-test") {
            auto other = spawn(root / L"other/oneproxy-tray.exe", L"--fixture", root / L"other-profile");
            HANDLE otherChild = waitChild(other.dwProcessId);
            auto target = spawn(root / L"target/oneproxy-tray.exe", L"--fixture", root / L"profile");
            HANDLE targetChild = waitChild(target.dwProcessId);
            prepared(root / L"target");
            require(WaitForSingleObject(target.hProcess, 0) == WAIT_OBJECT_0, "headless legacy fallback stops exact parent");
            require(WaitForSingleObject(targetChild, 0) == WAIT_OBJECT_0, "jobless verified child fallback leaves no orphan");
            require(WaitForSingleObject(other.hProcess, 0) == WAIT_TIMEOUT &&
                    WaitForSingleObject(otherChild, 0) == WAIT_TIMEOUT, "other installation and child unaffected");
            FinishUpgrade();
            HANDLE locked = CreateFileW((root / L"target/test.dll").c_str(), GENERIC_READ | GENERIC_WRITE,
                0, nullptr, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
            require(locked != INVALID_HANDLE_VALUE, "locked payload fixture created");
            require(PrepareUpgrade((root / L"target").c_str()) == 0, "locked DLL blocks preparation");
            CloseHandle(locked);
            prepared(root / L"target"); FinishUpgrade();
            require(!fs::exists(root / L"target/.oneproxy-upgrading"), "failure retry releases guard");
            SetFileAttributesW((root / L"target/test.dll").c_str(), FILE_ATTRIBUTE_READONLY);
            require(PrepareUpgrade((root / L"target").c_str()) == 0, "write access denied blocks preparation");
            SetFileAttributesW((root / L"target/test.dll").c_str(), FILE_ATTRIBUTE_NORMAL);
            auto independent = spawn(root / L"target/bin/sing-box.exe", L"--child", root / L"profile");
            require(PrepareUpgrade((root / L"target").c_str()) == 0, "unowned same-directory sing-box blocks overwrite");
            require(WaitForSingleObject(independent.hProcess, 0) == WAIT_TIMEOUT, "unowned sing-box is not killed");
            TerminateProcess(independent.hProcess, 0); WaitForSingleObject(independent.hProcess, 5000); CloseHandle(independent.hProcess);
            prepared(root / L"target"); FinishUpgrade();
            auto hung = spawn(root / L"target/oneproxy-tray.exe", L"--hung", root / L"profile");
            HANDLE hungChild = waitChild(hung.dwProcessId);
            require(trayThread(hung.dwProcessId) != 0, "cooperative-but-hung fixture ready");
            require(PrepareUpgrade((root / L"target").c_str()) == 0, "cooperative shutdown timeout blocks overwrite");
            require(WaitForSingleObject(hung.hProcess, 0) == WAIT_TIMEOUT, "cooperative timeout is not forcibly terminated");
            CloseHandle(hungChild); CloseHandle(hung.hProcess);
            CloseHandle(targetChild); CloseHandle(target.hProcess);
            // Only our cleanup job owns these deliberately surviving fixtures.
            CloseHandle(otherChild); CloseHandle(other.hProcess);
        } else return 2;
        FinishUpgrade(); CloseHandle(job); CloseDesktop(isolated);
        return 0;
    } catch (const std::exception &error) {
        std::cerr << "FAIL " << error.what() << std::endl;
        FinishUpgrade(); if (job) CloseHandle(job);
        return 1;
    }
}
