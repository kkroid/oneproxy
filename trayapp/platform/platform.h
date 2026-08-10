#pragma once

#include <QString>

class QSystemTrayIcon;

namespace Platform {

QString sharedLibraryName();

bool autoStartSupported();
bool autoStartEnabled();
bool setAutoStart(bool enabled);

bool routingModeSupported();
QString routingMode();
bool setRoutingMode(const QString &mode);

void initializeTrayRecovery(QSystemTrayIcon *tray);

} // namespace Platform
