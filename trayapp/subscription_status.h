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
        if (!status["subscription_configured"].toBool()) return;
        if (status["subscription_updating"].toBool()) {
            text = status["running"].toBool()
                ? QStringLiteral("Subscription updating...")
                : QStringLiteral("Subscription updating before connection...");
            return;
        }
        const QString result = status["subscription_result_id"].toString();
        if (!status["subscription_last_error"].toString().isEmpty()) {
            // Never display arbitrary error text, which may include feed credentials.
            text = status["running"].toBool()
                ? QStringLiteral("Subscription update failed. Please check the subscription and try again.")
                : QStringLiteral("Subscription update failed; not connected. Check your network/subscription and Start again to retry.");
            notifyFailure = !result.isEmpty() && result != lastResult;
        } else if (status["running"].toBool() && !status["subscription_last_at"].toString().isEmpty()) {
            text = QStringLiteral("Subscription is up to date");
        }
        if (!result.isEmpty()) lastResult = result;
    }

private:
    QString lastResult;
};
