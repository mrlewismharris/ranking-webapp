package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoConversionPlan(t *testing.T) {
	streams := []videoStream{{Index: 2, Type: "video", Codec: "h264", PixelFormat: "yuv420p", Width: 1920, Height: 1080}, {Index: 3, Type: "audio", Codec: "aac"}}
	args, err := conversionArgs("source", "target", streams)
	if err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(args, " ")
	if !strings.Contains(plan, "-map 0:2 -c:v copy -map 0:3 -c:a copy") || !strings.Contains(plan, "-movflags +faststart") {
		t.Fatal(plan)
	}
	streams[0].Codec, streams[0].Width, streams[1].Codec = "vp9", 1919, "opus"
	args, err = conversionArgs("source", "target", streams)
	if err != nil {
		t.Fatal(err)
	}
	plan = strings.Join(args, " ")
	for _, want := range []string{"-c:v libx264", "-crf 16", "-pix_fmt yuv420p", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-fps_mode passthrough", "-c:a aac"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("missing %s: %s", want, plan)
		}
	}
	if _, err := conversionArgs("source", "target", streams[1:]); err == nil {
		t.Fatal("accepted audio-only upload")
	}
	streams[0].Disposition.Attached = 1
	if _, err := conversionArgs("source", "target", streams); err == nil {
		t.Fatal("accepted cover art as video")
	}
}

func uploadVideoForTest(a *App, name string, data []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	f, _ := mw.CreateFormFile("file", name)
	f.Write(data)
	mw.Close()
	r := httptest.NewRequest("POST", "/api/upload", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.AddCookie(&http.Cookie{Name: "crown_session", Value: strings.Repeat("a", 48)})
	w := httptest.NewRecorder()
	a.middleware(http.HandlerFunc(a.upload)).ServeHTTP(w, r)
	return w
}

func TestVideoMissingToolsCleanup(t *testing.T) {
	a := testApp(t)
	t.Setenv("FFMPEG_PATH", filepath.Join(t.TempDir(), "missing-ffmpeg"))
	w := uploadVideoForTest(a, "clip.MOV", []byte("video candidate"))
	if w.Code != 503 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	assertNoVideoUploads(t, a)
}

func assertNoVideoUploads(t *testing.T, a *App) {
	t.Helper()
	var count int
	if err := a.db.QueryRow("SELECT count(*) FROM media").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed conversion was registered")
	}
	entries, err := os.ReadDir(filepath.Join(a.data, "uploads"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed upload left files: %v, %v", entries, err)
	}
}

func TestVideoUploadsFFmpeg(t *testing.T) {
	ffmpeg, err := videoTool("FFMPEG_PATH", "ffmpeg")
	if err != nil {
		t.Skip("FFmpeg unavailable")
	}
	ffprobe, err := videoTool("FFPROBE_PATH", "ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	run := func(args ...string) []byte {
		t.Helper()
		output, err := exec.Command(ffmpeg, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg: %v: %s", err, output)
		}
		return output
	}
	for _, fixture := range []struct{ name, codec, audio string }{
		{"compatible.mov", "libx264", "aac"},
		{"clip.webm", "libvpx-vp9", "libopus"},
		{"clip.avi", "mpeg4", "pcm_s16le"},
		{"clip.mkv", "mpeg4", "aac"},
		{"clip.mp4", "mpeg4", "aac"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			a := testApp(t)
			source := filepath.Join(t.TempDir(), fixture.name)
			run("-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x48:rate=10", "-f", "lavfi", "-i", "sine=frequency=440", "-t", "0.3", "-c:v", fixture.codec, "-pix_fmt", "yuv420p", "-c:a", fixture.audio, source)
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			w := uploadVideoForTest(a, fixture.name, data)
			if w.Code != 200 {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			var result map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["kind"] != "video" {
				t.Fatal(result)
			}
			var path string
			if err := a.db.QueryRow("SELECT path FROM media WHERE id=?", result["id"]).Scan(&path); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(a.data, path)
			streams, err := probeVideo(context.Background(), ffprobe, target)
			if err != nil {
				t.Fatal(err)
			}
			if len(streams) != 2 || streams[0].Codec != "h264" || streams[0].PixelFormat != "yuv420p" || streams[0].Width != 64 || streams[0].Height != 48 || streams[1].Codec != "aac" {
				t.Fatalf("unexpected output: %+v", streams)
			}
			if fixture.codec == "libx264" {
				// Compare the compressed video payload itself, proving remux introduces no loss.
				a := run("-v", "error", "-i", source, "-map", "0:v:0", "-c", "copy", "-f", "hash", "-")
				b := run("-v", "error", "-i", target, "-map", "0:v:0", "-c", "copy", "-f", "hash", "-")
				if !bytes.Equal(a, b) {
					t.Fatalf("remux changed video: %s != %s", a, b)
				}
			}
			r := httptest.NewRequest("GET", "/media/"+result["id"], nil)
			r.SetPathValue("id", result["id"])
			r.AddCookie(&http.Cookie{Name: "crown_session", Value: strings.Repeat("a", 48)})
			r.Header.Set("Range", "bytes=0-15")
			w = httptest.NewRecorder()
			a.middleware(http.HandlerFunc(a.media)).ServeHTTP(w, r)
			if w.Code != 206 || w.Header().Get("Content-Type") != "video/mp4" {
				t.Fatalf("media response: %d %s", w.Code, w.Header())
			}
			b, _ := io.ReadAll(w.Result().Body)
			if len(b) != 16 {
				t.Fatal("incorrect range")
			}
			entries, _ := os.ReadDir(filepath.Join(a.data, "uploads"))
			if len(entries) != 1 {
				t.Fatal("source or partial output left behind")
			}
		})
	}
	t.Run("invalid", func(t *testing.T) {
		a := testApp(t)
		if w := uploadVideoForTest(a, "fake.mkv", []byte("not a video")); w.Code != 400 {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		assertNoVideoUploads(t, a)
	})
	t.Run("audio only", func(t *testing.T) {
		a := testApp(t)
		source := filepath.Join(t.TempDir(), "audio.mp4")
		run("-v", "error", "-f", "lavfi", "-i", "sine=frequency=440", "-t", "0.1", "-c:a", "aac", source)
		data, _ := os.ReadFile(source)
		if w := uploadVideoForTest(a, "audio.mp4", data); w.Code != 400 {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		assertNoVideoUploads(t, a)
	})
}
