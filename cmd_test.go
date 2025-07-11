package gcmd

import (
	"context"
	"testing"
	"time"
)

func TestRunning(t *testing.T) {
	cmd := NewCmd("ffmpeg", "-stream_loop", "-1", "-re", "-i", "test.mp4", "-c", "copy", "-f", "flv", "rtmp://127.0.0.1/live/test")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-cmd.Start():
			t.Log("done")
			return
		case <-ticker.C:
			status := cmd.Status()
			t.Log(time.Now(), status.PID, status.Complete, status.LatestOut, status.LatestErr)
		}
	}
}

func TestStop(t *testing.T) {
	cmd := NewCmd("ffmpeg", "-stream_loop", "-1", "-re", "-i", "test.mp4", "-c", "copy", "-f", "flv", "rtmp://127.0.0.1/live/test")
	after := time.After(time.Second * 10)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-after:
			_ = cmd.Stop()
		case <-cmd.Start():
			t.Log("done")
			return
		case <-ticker.C:
			status := cmd.Status()
			t.Logf("PID: %d, Complete: %t, LatestOut: %s, LatestErr: %s", status.PID, status.Complete, status.LatestOut, status.LatestErr)
		}
	}
}

func TestWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	cmd := NewCmdWithCtx(ctx, "ffmpeg", "-stream_loop", "-1", "-re", "-i", "test.mp4", "-c", "copy", "-f", "flv", "rtmp://127.0.0.1/live/test")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-cmd.Start():
			t.Log("done")
			return
		case <-ticker.C:
			status := cmd.Status()
			t.Logf("PID: %d, Complete: %t, LatestOut: %s, LatestErr: %s", status.PID, status.Complete, status.LatestOut, status.LatestErr)
		}
	}
}
