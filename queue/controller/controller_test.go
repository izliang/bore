package controller_test

import (
	"testing"
	"time"

	"github.com/izliang/bore/queue/controller"
)

func TestStaticController_Resolve(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{
		"q1": 5,
		"q2": 10,
	})

	if v := ctrl.Resolve("q1"); v != 5 {
		t.Errorf("q1 = %d, want 5", v)
	}
	if v := ctrl.Resolve("q2"); v != 10 {
		t.Errorf("q2 = %d, want 10", v)
	}
	if v := ctrl.Resolve("unknown"); v != 0 {
		t.Errorf("unknown = %d, want 0", v)
	}
}

func TestStaticController_Set(t *testing.T) {
	ctrl := controller.NewStatic(nil)

	ctrl.Set("q1", 3)
	if v := ctrl.Resolve("q1"); v != 3 {
		t.Errorf("after Set, q1 = %d, want 3", v)
	}

	ctrl.Set("q1", 7)
	if v := ctrl.Resolve("q1"); v != 7 {
		t.Errorf("after second Set, q1 = %d, want 7", v)
	}
}

func TestStaticController_WatchNotified(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 1})

	ch, stop := ctrl.Watch("q")
	defer stop()

	ctrl.Set("q", 5)

	select {
	case v := <-ch:
		if v != 5 {
			t.Errorf("received %d, want 5", v)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestStaticController_WatchLatestOnly(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 1})

	ch, stop := ctrl.Watch("q")
	defer stop()

	// Rapid consecutive sets: only the latest value should be delivered.
	ctrl.Set("q", 2)
	ctrl.Set("q", 3)
	ctrl.Set("q", 10)

	select {
	case v := <-ch:
		if v != 10 {
			t.Errorf("received %d, want 10 (latest)", v)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}

	// Channel should be empty after reading the latest.
	select {
	case v := <-ch:
		t.Errorf("unexpected extra value: %d", v)
	default:
	}
}

func TestStaticController_StopUnsubscribes(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 1})

	ch, stop := ctrl.Watch("q")
	stop()

	// Set after stop: channel should not receive (drain-and-replace
	// only replaces in the subs slice, but we've been removed).
	ctrl.Set("q", 99)

	select {
	case v := <-ch:
		t.Errorf("received %d after stop, want nothing", v)
	default:
	}
}

func TestStaticController_MultipleWatchers(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 1})

	ch1, stop1 := ctrl.Watch("q")
	defer stop1()
	ch2, stop2 := ctrl.Watch("q")
	defer stop2()

	ctrl.Set("q", 8)

	for i, ch := range []<-chan int{ch1, ch2} {
		select {
		case v := <-ch:
			if v != 8 {
				t.Errorf("watcher %d: received %d, want 8", i, v)
			}
		case <-time.After(time.Second):
			t.Fatalf("watcher %d: timed out", i)
		}
	}
}

func TestStaticController_SetNeverBlocks(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 1})

	// Create a watcher but never read from it.
	_, stop := ctrl.Watch("q")
	defer stop()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			ctrl.Set("q", i)
		}
		close(done)
	}()

	select {
	case <-done:
		// Set never blocked — success.
	case <-time.After(2 * time.Second):
		t.Fatal("Set blocked when watcher buffer was full")
	}
}
