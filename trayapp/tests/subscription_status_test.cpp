#include "../subscription_status.h"
#include <QtTest>

class SubscriptionStatusTest : public QObject {
    Q_OBJECT
private slots:
    void progressResultsAndNotifications() {
        SubscriptionStatus view;
        QJsonObject status{{"running", true}, {"subscription_configured", true},
                           {"subscription_updating", true}};
        view.update(status);
        QVERIFY(view.text.contains("updating"));
        QVERIFY(!view.notifyFailure);
        status["subscription_updating"] = false;
        status["subscription_last_error"] = "https://secret:credential@example.test/?token=private";
        status["subscription_result_id"] = "1";
        view.update(status);
        QVERIFY(view.notifyFailure);
        QVERIFY(!view.text.contains("secret"));
        QVERIFY(!view.text.contains("private"));
        view.update(status);
        QVERIFY(!view.notifyFailure);
        // Even if the next request starts and finishes between polls, its ID differs.
        status["subscription_result_id"] = "2";
        view.update(status);
        QVERIFY(view.notifyFailure);
        status["running"] = false;
        view.update(status);
        QVERIFY(view.text.isEmpty());
        QVERIFY(!view.notifyFailure);
        status["running"] = true;
        status["subscription_configured"] = false;
        view.update(status);
        QVERIFY(view.text.isEmpty());
        QVERIFY(!view.notifyFailure);
        status["subscription_configured"] = true;
        status["subscription_last_error"] = "";
        status["subscription_last_at"] = "2026-10-11T12:00:00Z";
        status["subscription_result_id"] = "3";
        view.update(status);
        QVERIFY(view.text.contains("up to date"));
        QVERIFY(!view.notifyFailure);
    }
};

QTEST_GUILESS_MAIN(SubscriptionStatusTest)
#include "subscription_status_test.moc"
