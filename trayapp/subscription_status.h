#pragma once

#include <QJsonObject>
#include <QString>

class SubscriptionStatus {
public:
    QString text;
    bool notifyFailure = false;

    void update(const QJsonObject &status) {
        text.clear();
        notifyFailure = false;
        if (!status["running"].toBool() || !status["subscription_configured"].toBool()) return;
        if (status["subscription_updating"].toBool()) {
            text = QStringLiteral("Subscription updating...");
            return;
        }
        const QString result = status["subscription_result_id"].toString();
        if (!status["subscription_last_error"].toString().isEmpty()) {
            // Never display arbitrary error text, which may include feed credentials.
            text = QStringLiteral("Subscription update failed. Please check the subscription and try again.");
            notifyFailure = !result.isEmpty() && result != lastResult;
        } else if (!status["subscription_last_at"].toString().isEmpty()) {
            text = QStringLiteral("Subscription is up to date");
        }
        if (!result.isEmpty()) lastResult = result;
    }

private:
    QString lastResult;
};
