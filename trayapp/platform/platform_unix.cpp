#include "platform.h"

namespace Platform {

QString sharedLibraryName()
{
#ifdef Q_OS_MACOS
    return QStringLiteral("liboneproxy.dylib");
#else
    return QStringLiteral("liboneproxy.so");
#endif
}

bool autoStartSupported()
{
    return false;
}

bool autoStartEnabled()
{
    return false;
}

bool setAutoStart(bool)
{
    return false;
}

bool routingModeSupported()
{
    return false;
}

QString routingMode()
{
    return QStringLiteral("global");
}

bool setRoutingMode(const QString &)
{
    return false;
}

void initializeTrayRecovery(QSystemTrayIcon *)
{
}

} // namespace Platform
