#include "../platform/platform.h"
#include "../platform/autostart_settings.h"

#include <QtTest>

class PlatformTest : public QObject
{
    Q_OBJECT

private slots:
    void sharedLibraryNameMatchesPlatform();
    void autoStartCapabilityMatchesPlatform();
    void routingModeCapabilityMatchesPlatform();
    void autoStartOwnershipAndOptIn();
    void autoStartWriteFailure();
};

void PlatformTest::sharedLibraryNameMatchesPlatform()
{
#ifdef Q_OS_WIN
    QCOMPARE(Platform::sharedLibraryName(), QStringLiteral("oneproxy.dll"));
#elif defined(Q_OS_MACOS)
    QCOMPARE(Platform::sharedLibraryName(), QStringLiteral("liboneproxy.dylib"));
#else
    QCOMPARE(Platform::sharedLibraryName(), QStringLiteral("liboneproxy.so"));
#endif
}

void PlatformTest::autoStartCapabilityMatchesPlatform()
{
#ifdef Q_OS_WIN
    QVERIFY(Platform::autoStartSupported());
#else
    QVERIFY(!Platform::autoStartSupported());
#endif
}

void PlatformTest::routingModeCapabilityMatchesPlatform()
{
#ifdef Q_OS_WIN
    QVERIFY(Platform::routingModeSupported());
#else
    QVERIFY(!Platform::routingModeSupported());
    QVERIFY(!Platform::setRoutingMode(QStringLiteral("rule")));
#endif
}

void PlatformTest::autoStartOwnershipAndOptIn()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    QSettings settings(directory.filePath("startup.ini"), QSettings::IniFormat);
    const QString executable = QStringLiteral("C:/Program Files/OneProxy/oneproxy-tray.exe");
    QVERIFY(!settings.contains("OneProxy"));
    QVERIFY(!AutoStart::belongsTo(settings.value("OneProxy").toString(), executable));
    settings.setValue("OtherApp", "keep-me");
    QVERIFY(AutoStart::setEnabled(settings, executable, true));
    QVERIFY(settings.value("OneProxy").toString().startsWith('"'));
    QVERIFY(AutoStart::belongsTo(settings.value("OneProxy").toString(), executable));
    QVERIFY(AutoStart::belongsTo(executable.toUpper(), executable)); // Legacy unquoted entry.
    QVERIFY(!AutoStart::belongsTo(AutoStart::command(executable) + " --other", executable));
    const QString other = QStringLiteral("C:/Other/oneproxy-tray.exe");
    settings.setValue("OneProxy", other);
    QVERIFY(!AutoStart::setEnabled(settings, executable, false));
    QVERIFY(!AutoStart::setEnabled(settings, executable, true));
    QCOMPARE(settings.value("OneProxy").toString(), other);
    settings.setValue("OneProxy", executable);
    QVERIFY(AutoStart::setEnabled(settings, executable, false));
    QVERIFY(!settings.contains("OneProxy"));
    QCOMPARE(settings.value("OtherApp").toString(), QStringLiteral("keep-me"));
    QVERIFY(AutoStart::setEnabled(settings, executable, false));
}

void PlatformTest::autoStartWriteFailure()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    // A directory cannot be replaced with an INI file; no real Run key is touched.
    QSettings settings(directory.path(), QSettings::IniFormat);
    QVERIFY(!AutoStart::setEnabled(settings, QStringLiteral("C:/OneProxy/tray.exe"), true));
    QVERIFY(settings.status() != QSettings::NoError);
}

QTEST_MAIN(PlatformTest)

#include "platform_test.moc"
