#pragma once

#include <QLocalServer>
#include <QString>
#include <functional>

// The caller holds InstanceGuard while listening or replacing the bundle.
bool listenForUpgrade(QLocalServer &server, const QString &stateDirectory,
                      const std::function<QString()> &stop);
int installMacUpdate(const QString &source, const QString &destination,
                     const QString &stateDirectory);
