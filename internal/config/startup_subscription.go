package config

import (
	"context"
	"errors"
)

const StartupSubscriptionError = "Subscription update failed; not connected. Check your network/subscription and Start again to retry."

// PrepareStartupSubscription never mutates or saves current. The caller commits
// the validated result only while it still owns the startup session.
func PrepareStartupSubscription(ctx context.Context, current *Config) (*Config, error) {
	if current.SubscriptionURL == "" {
		return current, nil
	}
	incoming, _, err := FetchSubscriptionContext(ctx, current.SubscriptionURL, 0)
	if err != nil {
		return nil, errors.New(StartupSubscriptionError)
	}
	next, _, err := MergeSubscription(current, incoming, current.SubscriptionURL)
	if err != nil || ctx.Err() != nil {
		return nil, errors.New(StartupSubscriptionError)
	}
	return next, nil
}
