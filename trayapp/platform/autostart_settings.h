#pragma once

#include <QDir>
#include <QSettings>

namespace AutoStart {

inline QString command(const QString &executable)
{
    return QStringLiteral("\"") + QDir::toNativeSeparators(executable) + QStringLiteral("\"");
}

inline bool belongsTo(const QString &value, const QString &executable)
{
    QString path = value;
    if (path.startsWith(QLatin1Char('"')) && path.endsWith(QLatin1Char('"')))
        path = path.mid(1, path.size() - 2);
    return !path.isEmpty() &&
        QDir::fromNativeSeparators(path).compare(
            QDir::fromNativeSeparators(executable), Qt::CaseInsensitive) == 0;
}

inline bool setEnabled(QSettings &settings, const QString &executable, bool enabled)
{
    const QString name = QStringLiteral("OneProxy");
    settings.sync();
    if (settings.status() != QSettings::NoError)
        return false;
    const QString current = settings.value(name).toString();
    // A different installation or another program owns this value: preserve it.
    if (settings.contains(name) && !belongsTo(current, executable))
        return false;
    if (enabled)
        settings.setValue(name, command(executable));
    else
        settings.remove(name);
    settings.sync();
    return settings.status() == QSettings::NoError &&
        (enabled ? settings.value(name).toString() == command(executable)
                 : !settings.contains(name));
}

} // namespace AutoStart
