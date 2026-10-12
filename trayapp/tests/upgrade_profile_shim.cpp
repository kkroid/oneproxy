// TEST ONLY. Qt 6.8's homePath uses the token profile ahead of USERPROFILE.
// Redirect only QtCore's GetUserProfileDirectoryW import inside our suspended
// fixture. No release file is patched; no registry or real profile is modified.
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <cstring>

BOOL WINAPI isolatedProfile(HANDLE, LPWSTR buffer, LPDWORD count)
{
    DWORD required = GetEnvironmentVariableW(L"USERPROFILE", nullptr, 0);
    if (!required) return FALSE;
    if (!buffer || *count < required) {
        *count = required; SetLastError(ERROR_INSUFFICIENT_BUFFER); return FALSE;
    }
    GetEnvironmentVariableW(L"USERPROFILE", buffer, *count);
    *count = required;
    return TRUE;
}
BOOL WINAPI DllMain(HINSTANCE, DWORD reason, LPVOID)
{
    if (reason != DLL_PROCESS_ATTACH) return TRUE;
    auto base = reinterpret_cast<BYTE *>(GetModuleHandleW(L"Qt6Core.dll"));
    if (!base) return FALSE;
    auto dos = reinterpret_cast<IMAGE_DOS_HEADER *>(base);
    auto nt = reinterpret_cast<IMAGE_NT_HEADERS *>(base + dos->e_lfanew);
    auto imports = reinterpret_cast<IMAGE_IMPORT_DESCRIPTOR *>(base +
        nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT].VirtualAddress);
    for (; imports->Name; ++imports) {
        auto names = reinterpret_cast<IMAGE_THUNK_DATA *>(base + imports->OriginalFirstThunk);
        auto addresses = reinterpret_cast<IMAGE_THUNK_DATA *>(base + imports->FirstThunk);
        for (; names->u1.AddressOfData; ++names, ++addresses) {
            if (IMAGE_SNAP_BY_ORDINAL(names->u1.Ordinal)) continue;
            auto name = reinterpret_cast<IMAGE_IMPORT_BY_NAME *>(base + names->u1.AddressOfData);
            if (std::strcmp(reinterpret_cast<char *>(name->Name), "GetUserProfileDirectoryW")) continue;
            DWORD protection;
            if (!VirtualProtect(&addresses->u1.Function, sizeof(ULONG_PTR), PAGE_READWRITE, &protection)) return FALSE;
            addresses->u1.Function = reinterpret_cast<ULONG_PTR>(&isolatedProfile);
            VirtualProtect(&addresses->u1.Function, sizeof(ULONG_PTR), protection, &protection);
            return TRUE;
        }
    }
    return FALSE;
}
