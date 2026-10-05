package realtime

// Routing invariants: personal events reach exactly their user; org events
// reach that org's members (plus platform admins); admin-only events reach
// only admins; unrouted events reach everyone.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func collect(t *testing.T, hub *Hub, sub Subscriber, publish func()) []Event {
	t.Helper()
	events, cancel := hub.Subscribe(sub)
	defer cancel()
	publish()
	// Deterministic drain: Publish is synchronous under the hood, so
	// everything already in the channel is readable immediately.
	var got []Event
	for {
		select {
		case ev := <-events:
			got = append(got, ev)
			continue
		default:
		}
		return got
	}
}

func hasType(events []Event, eventType string) bool {
	for _, ev := range events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func TestHubRoutingScopes(t *testing.T) {
	hub := NewHub()
	orgMember := Subscriber{UserID: "u1", Orgs: []string{"org-a"}}
	otherOrg := Subscriber{UserID: "u2", Orgs: []string{"org-b"}}
	admin := Subscriber{UserID: "u3", Orgs: []string{"org-b"}, Admin: true, CanReadJobs: true}

	t.Run("org event stays inside its org and reaches admins", func(t *testing.T) {
		ev := NewOrgEvent(TypeTicketUpdated, map[string]string{"title": "secret"}, "org-a")
		if got := collect(t, hub, orgMember, func() { hub.Publish(ev) }); !hasType(got, TypeTicketUpdated) {
			t.Fatal("org member must receive their org's events")
		}
		if got := collect(t, hub, otherOrg, func() { hub.Publish(ev) }); hasType(got, TypeTicketUpdated) {
			t.Fatal("foreign-org member must NOT receive the event")
		}
		if got := collect(t, hub, admin, func() { hub.Publish(ev) }); !hasType(got, TypeTicketUpdated) {
			t.Fatal("platform admin must receive org events")
		}
	})

	t.Run("personal event reaches only its user", func(t *testing.T) {
		ev := NewUserEvent(TypeNotification, map[string]string{"body": "private"}, "u1")
		if got := collect(t, hub, orgMember, func() { hub.Publish(ev) }); !hasType(got, TypeNotification) {
			t.Fatal("target user must receive their notification")
		}
		for _, sub := range []Subscriber{otherOrg, admin} {
			if got := collect(t, hub, sub, func() { hub.Publish(ev) }); hasType(got, TypeNotification) {
				t.Fatalf("user %s must not receive someone else's notification", sub.UserID)
			}
		}
	})

	t.Run("admin-only event reaches only admins", func(t *testing.T) {
		ev := NewAdminEvent(TypeAnalyticsEvent, map[string]string{"name": "user_logged_in"})
		if got := collect(t, hub, admin, func() { hub.Publish(ev) }); !hasType(got, TypeAnalyticsEvent) {
			t.Fatal("admin must receive admin-only events")
		}
		for _, sub := range []Subscriber{orgMember, otherOrg} {
			if got := collect(t, hub, sub, func() { hub.Publish(ev) }); hasType(got, TypeAnalyticsEvent) {
				t.Fatalf("non-admin %s must not receive admin-only events", sub.UserID)
			}
		}
	})

	t.Run("unrouted events reach everyone authenticated", func(t *testing.T) {
		ev := NewEvent(TypeConnected, map[string]string{"at": "now"})
		for _, sub := range []Subscriber{orgMember, otherOrg, admin} {
			if got := collect(t, hub, sub, func() { hub.Publish(ev) }); !hasType(got, TypeConnected) {
				t.Fatalf("connected event must reach %s", sub.UserID)
			}
		}
	})
}

func TestWireStripsRouting(t *testing.T) {
	ev := NewOrgEvent(TypeTicketUpdated, map[string]string{"title": "t"}, "org-a")
	raw := string(mustJSON(t, ev))
	if !strings.Contains(raw, `"route"`) {
		t.Fatalf("full event should carry the route over the bridge: %s", raw)
	}
	wireRaw := string(mustJSON(t, ev.Wire()))
	if strings.Contains(wireRaw, "route") || strings.Contains(wireRaw, "org-a") {
		t.Fatalf("wire view leaked routing scope: %s", wireRaw)
	}
}

func TestFanSinkRoutesActivityByOrg(t *testing.T) {
	hub := NewHub()
	// The resolver answers org-a for the actor.
	sink := NewFanSink(hub).WithOrgResolver(func(context.Context, string) []string { return []string{"org-a"} })
	member := Subscriber{UserID: "u1", Orgs: []string{"org-a"}}
	outsider := Subscriber{UserID: "u2", Orgs: []string{"org-b"}}

	if got := collect(t, hub, member, func() {
		sink.AddEvent(context.Background(), "user_logged_in", "actor-1")
	}); !hasType(got, TypeAnalyticsEvent) {
		t.Fatal("actor's org member must see the activity event")
	}
	if got := collect(t, hub, outsider, func() {
		sink.AddEvent(context.Background(), "user_logged_in", "actor-1")
	}); hasType(got, TypeAnalyticsEvent) {
		t.Fatal("foreign-org user must not see the activity event")
	}

	// Without a resolver the event degrades to admin-only, never global.
	hub2 := NewHub()
	plain := NewFanSink(hub2)
	if got := collect(t, hub2, member, func() {
		plain.AddEvent(context.Background(), "user_logged_in", "actor-1")
	}); hasType(got, TypeAnalyticsEvent) {
		t.Fatal("unresolvable activity must not fan out to everyone")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
