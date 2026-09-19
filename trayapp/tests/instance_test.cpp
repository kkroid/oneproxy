#include "../instance_guard.h"
#include <QtTest>
#include <QProcess>
#include <QTemporaryDir>

class InstanceTest : public QObject {
    Q_OBJECT
private:
    QString helper() const {
#ifdef Q_OS_WIN
        return QCoreApplication::applicationDirPath() + "/oneproxy-instance-helper.exe";
#else
        return QCoreApplication::applicationDirPath() + "/oneproxy-instance-helper";
#endif
    }
    bool start(QProcess &process, const QString &program, const QString &mode, const QString &dir) {
        process.start(program, {mode, dir});
        if (!process.waitForStarted(5000) || !process.waitForReadyRead(5000)) return false;
        return process.readAllStandardOutput().contains("READY");
    }
private slots:
    void duplicateAcrossInstallPathsAndCrashRecovery() {
        QTemporaryDir state, otherInstall;
        QVERIFY(state.isValid());
        QVERIFY(otherInstall.isValid());
        const QString second = otherInstall.path() + "/" + QFileInfo(helper()).fileName();
        QVERIFY(QFile::copy(helper(), second));
        QProcess first;
        QVERIFY(start(first, helper(), "--hold", state.path()));
        QProcess duplicate;
        duplicate.start(second, {"--hold", state.path()});
        QVERIFY(duplicate.waitForFinished(5000));
        QCOMPARE(duplicate.exitCode(), 2);
        QVERIFY(duplicate.readAllStandardError().contains("already running"));
        QCOMPARE(first.state(), QProcess::Running);
        // Kill only our isolated lock fixture; no proxy is ever launched.
        first.kill();
        QVERIFY(first.waitForFinished(5000));
        QProcess recovered;
        QVERIFY(start(recovered, second, "--hold", state.path()));
        recovered.kill();
        QVERIFY(recovered.waitForFinished(5000));
    }
    void normalReleaseAndIndependentUsers() {
        QTemporaryDir firstUser, secondUser;
        QVERIFY(firstUser.isValid());
        QVERIFY(secondUser.isValid());
        {
            InstanceGuard first(firstUser.path()), second(secondUser.path());
            QVERIFY(first.acquire().isEmpty());
            QVERIFY(second.acquire().isEmpty());
        }
        InstanceGuard next(firstUser.path());
        QVERIFY(next.acquire().isEmpty());
    }
    void stateDirectoryFailureIsNotDuplicate() {
        QTemporaryDir temp;
        QFile file(temp.filePath("file"));
        QVERIFY(file.open(QIODevice::WriteOnly));
        file.close();
        InstanceGuard guard(file.fileName() + "/child");
        QVERIFY(guard.acquire().contains("Cannot create"));
    }
    void findsOldTrayWithoutTouchingIt() {
#ifdef Q_OS_WIN
        QTemporaryDir temp;
        QProcess oldTray;
        QVERIFY(start(oldTray, helper(), "--legacy", temp.path()));
        const auto pid = oldTray.processId();
        QVERIFY(legacyTrayProcesses().contains(pid));
        QCOMPARE(oldTray.state(), QProcess::Running);
        oldTray.kill();
        QVERIFY(oldTray.waitForFinished(5000));
        QVERIFY(!legacyTrayProcesses().contains(pid));
#else
        QSKIP("Legacy Windows message window compatibility");
#endif
    }
};

QTEST_GUILESS_MAIN(InstanceTest)
#include "instance_test.moc"
