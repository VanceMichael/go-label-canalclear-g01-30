package notification

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type Channel string

const (
	ChannelWebhook Channel = "webhook"
	ChannelEmail   Channel = "email"
)

type Event struct {
	ID         string
	TenantID   string
	Topic      string
	ObjectID   string
	OccurredAt time.Time
	Attributes map[string]string
}

type Subscription struct {
	ID             string
	TenantID       string
	Channel        Channel
	Destination    string
	Topics         []string
	AttributeMatch map[string]string
	Enabled        bool
	CreatedAt      time.Time
}

type Delivery struct {
	ID             string
	EventID        string
	SubscriptionID string
	Channel        Channel
	Destination    string
	Attempts       int
	AvailableAt    time.Time
}

func ValidateSubscription(subscription Subscription) error {
	if subscription.ID == "" || subscription.TenantID == "" || subscription.Destination == "" || subscription.CreatedAt.IsZero() {
		return fmt.Errorf("%w: notification subscription", domain.ErrInvalid)
	}
	if len(subscription.Topics) == 0 || len(subscription.Topics) > 50 {
		return fmt.Errorf("%w: subscription topics", domain.ErrInvalid)
	}
	switch subscription.Channel {
	case ChannelWebhook:
		if !strings.HasPrefix(subscription.Destination, "https://") {
			return fmt.Errorf("%w: webhook destination", domain.ErrInvalid)
		}
	case ChannelEmail:
		if !strings.Contains(subscription.Destination, "@") {
			return fmt.Errorf("%w: email destination", domain.ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: notification channel", domain.ErrInvalid)
	}
	seen := make(map[string]struct{}, len(subscription.Topics))
	for _, topic := range subscription.Topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return fmt.Errorf("%w: empty notification topic", domain.ErrInvalid)
		}
		if _, duplicate := seen[topic]; duplicate {
			return fmt.Errorf("%w: duplicate notification topic", domain.ErrConflict)
		}
		seen[topic] = struct{}{}
	}
	return nil
}

func Plan(event Event, subscriptions []Subscription, now time.Time) ([]Delivery, error) {
	if event.ID == "" || event.TenantID == "" || event.Topic == "" || event.OccurredAt.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("%w: notification event", domain.ErrInvalid)
	}
	result := make([]Delivery, 0)
	for _, subscription := range subscriptions {
		if err := ValidateSubscription(subscription); err != nil {
			return nil, err
		}
		if !subscription.Enabled || subscription.TenantID != event.TenantID || !matchesTopic(event.Topic, subscription.Topics) || !matchesAttributes(event.Attributes, subscription.AttributeMatch) {
			continue
		}
		result = append(result, Delivery{
			ID: event.ID + "-" + subscription.ID, EventID: event.ID, SubscriptionID: subscription.ID,
			Channel: subscription.Channel, Destination: subscription.Destination, AvailableAt: now.UTC(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func matchesTopic(topic string, subscriptions []string) bool {
	for _, candidate := range subscriptions {
		if candidate == topic || strings.HasSuffix(candidate, ".*") && strings.HasPrefix(topic, strings.TrimSuffix(candidate, "*")) {
			return true
		}
	}
	return false
}

func matchesAttributes(attributes, required map[string]string) bool {
	for key, value := range required {
		if attributes[key] != value {
			return false
		}
	}
	return true
}

func Retry(delivery Delivery, now time.Time, backoff time.Duration, maximumAttempts int) (Delivery, bool, error) {
	if delivery.ID == "" || now.IsZero() || backoff <= 0 || maximumAttempts <= 0 {
		return delivery, false, fmt.Errorf("%w: delivery retry", domain.ErrInvalid)
	}
	out := delivery
	out.Attempts++
	if out.Attempts >= maximumAttempts {
		return out, false, nil
	}
	out.AvailableAt = now.UTC().Add(backoff)
	return out, true, nil
}
