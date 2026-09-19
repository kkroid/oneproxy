#pragma once

#include <QLockFile>
#include <QString>
#include <QList>

// Owns the lock for the entire tray lifetime, including core shutdown.
class InstanceGuard {
public:
    explicit InstanceGuard(const QString &stateDirectory);
    QString acquire();

private:
    QString directory;
    QLockFile lock;
};

// Older Windows releases have no lock but expose this message-only window.
// Discovery is read-only and limited to the current desktop.
QList<qint64> legacyTrayProcesses();
