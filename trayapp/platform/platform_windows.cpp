#include "platform.h"

#include <QCoreApplication>
#include <QSettings>
#include <QSystemTrayIcon>
#include <qt_windows.h>

namespace {

const QString autoStartRegistryPath = QStringLiteral(
    "HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run");
const QString routeRegistryPath = QStringLiteral(
    "HKEY_CURRENT_USER\\Software\\OneProxy");
const QString autoStartValueName = QStringLiteral("OneProxy");
const QString routeModeValueName = QStringLiteral("RouteMode");

UINT taskbarCreatedMessage = 0;
QSystemTrayIcon *trayIcon = nullptr;

LRESULT CALLBACK trayWindowProc(HWND window, UINT message, WPARAM wParam, LPARAM lParam)
{
    if (message == taskbarCreatedMessage && trayIcon) {
        trayIcon->hide();
        trayIcon->show();
    }
    return DefWindowProcW(window, message, wParam, lParam);
}

} // namespace

namespace Platform {

QString sharedLibraryName()
{
    return QStringLiteral("oneproxy.dll");
}

bool autoStartSupported()
{
    return true;
}

bool autoStartEnabled()
{
    const QSettings settings(autoStartRegistryPath, QSettings::NativeFormat);
    return !settings.value(autoStartValueName).toString().isEmpty();
}

bool setAutoStart(bool enabled)
{
    QSettings settings(autoStartRegistryPath, QSettings::NativeFormat);
    if (enabled) {
        settings.setValue(autoStartValueName, QCoreApplication::applicationFilePath());
    } else {
        settings.remove(autoStartValueName);
    }
    return settings.status() == QSettings::NoError;
}

bool routingModeSupported()
{
    return true;
}

QString routingMode()
{
    const QSettings settings(routeRegistryPath, QSettings::NativeFormat);
    const QString mode = settings.value(routeModeValueName).toString();
    return mode.isEmpty() ? QStringLiteral("global") : mode;
}

bool setRoutingMode(const QString &mode)
{
    QSettings settings(routeRegistryPath, QSettings::NativeFormat);
    settings.setValue(routeModeValueName, mode);
    return settings.status() == QSettings::NoError;
}

void initializeTrayRecovery(QSystemTrayIcon *tray)
{
    trayIcon = tray;
    taskbarCreatedMessage = RegisterWindowMessageW(L"TaskbarCreated");

    WNDCLASSEXW windowClass = {};
    windowClass.cbSize = sizeof(windowClass);
    windowClass.lpfnWndProc = trayWindowProc;
    windowClass.hInstance = GetModuleHandleW(nullptr);
    windowClass.lpszClassName = L"OneProxyTrayWindow";
    RegisterClassExW(&windowClass);

    CreateWindowExW(0, L"OneProxyTrayWindow", L"", 0, 0, 0, 0, 0,
                    HWND_MESSAGE, nullptr, GetModuleHandleW(nullptr), nullptr);
}

} // namespace Platform
