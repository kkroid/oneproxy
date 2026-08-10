#include "../platform/platform.h"

#include <QtTest>

class PlatformTest : public QObject
{
    Q_OBJECT

private slots:
    void sharedLibraryNameMatchesPlatform();
    void autoStartCapabilityMatchesPlatform();
    void routingModeCapabilityMatchesPlatform();
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

QTEST_MAIN(PlatformTest)

#include "platform_test.moc"
