package gcmd

import (
	"context"
	"testing"
	"time"
)

func TestCl(t *testing.T) {
	cmd := NewCmd("ffmpeg", "-re", "-i", "test.mp4", "-c", "copy", "-f", "flv", "test.flv", "-y")
	after := time.After(time.Second * 3)
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
			t.Logf("%+v\n", status)
		}
	}
}

func TestClCtx(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	cmd := NewCmdWithCtx(ctx, "ffmpeg", "-re", "-i", "test.mp4", "-c", "copy", "-f", "flv", "test.flv", "-y")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-cmd.Start():
			t.Log("done")
			return
		case <-ticker.C:
			status := cmd.Status()
			t.Logf("%+v\n", status)
		}
	}
}
