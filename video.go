package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	errVideoTools   = errors.New("video conversion tools unavailable")
	errInvalidVideo = errors.New("invalid or unsupported video")
	videoSlots      = make(chan struct{}, 2)
)

// Limit demuxers as well as protocols: uploaded playlists must not load other files.
const videoFormats = "mov,matroska,webm,avi,mpeg,mpegvideo,mpegts,asf,flv,ogg"

func isVideoUpload(name, mime string) bool {
	if strings.HasPrefix(mime, "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mov", ".mkv", ".webm", ".avi", ".m4v", ".mpg", ".mpeg", ".wmv", ".flv", ".ts", ".mts", ".m2ts", ".3gp", ".3g2", ".ogv":
		return true
	}
	return false
}

type videoStream struct {
	Index       int    `json:"index"`
	Type        string `json:"codec_type"`
	Codec       string `json:"codec_name"`
	PixelFormat string `json:"pix_fmt"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Disposition struct {
		Attached int `json:"attached_pic"`
	} `json:"disposition"`
}

func videoTool(env, fallback string) (string, error) {
	path := os.Getenv(env)
	if path == "" {
		path = fallback
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", errVideoTools, fallback)
	}
	return resolved, nil
}

func probeVideo(ctx context.Context, tool, source string) ([]videoStream, error) {
	cmd := exec.CommandContext(ctx, tool, "-v", "error", "-protocol_whitelist", "file", "-format_whitelist", videoFormats,
		"-show_entries", "stream=index,codec_type,codec_name,pix_fmt,width,height:stream_disposition=attached_pic", "-of", "json", source)
	data, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidVideo, err)
	}
	var result struct {
		Streams []videoStream `json:"streams"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidVideo, err)
	}
	return result.Streams, nil
}

func conversionArgs(source, target string, streams []videoStream) ([]string, error) {
	var video, audio *videoStream
	for i := range streams {
		s := &streams[i]
		if s.Type == "video" && s.Disposition.Attached == 0 && video == nil {
			video = s
		}
		if s.Type == "audio" && audio == nil {
			audio = s
		}
	}
	if video == nil || video.Width <= 0 || video.Height <= 0 {
		return nil, errInvalidVideo
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-protocol_whitelist", "file", "-format_whitelist", videoFormats,
		"-i", source, "-map", fmt.Sprintf("0:%d", video.Index)}
	// Stream copy preserves the compressed video exactly when already web-compatible.
	if video.Codec == "h264" && video.PixelFormat == "yuv420p" && video.Width%2 == 0 && video.Height%2 == 0 {
		args = append(args, "-c:v", "copy")
	} else {
		// Pad odd dimensions by at most one pixel; never scale down or force a frame rate.
		args = append(args, "-c:v", "libx264", "-preset", "slow", "-crf", "16", "-pix_fmt", "yuv420p",
			"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-fps_mode", "passthrough", "-threads", "2")
	}
	if audio != nil {
		args = append(args, "-map", fmt.Sprintf("0:%d", audio.Index))
		if audio.Codec == "aac" {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "256k")
		}
	}
	return append(args, "-movflags", "+faststart", "-f", "mp4", target), nil
}

func convertVideo(parent context.Context, source, target string) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	select {
	case videoSlots <- struct{}{}:
		defer func() { <-videoSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	ffmpeg, err := videoTool("FFMPEG_PATH", "ffmpeg")
	if err != nil {
		return err
	}
	ffprobe, err := videoTool("FFPROBE_PATH", "ffprobe")
	if err != nil {
		return err
	}
	streams, err := probeVideo(ctx, ffprobe, source)
	if err != nil {
		return err
	}
	args, err := conversionArgs(source, target, streams)
	if err != nil {
		return err
	}
	// Stderr stays bounded even for malformed files with repeated decoder errors.
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stderr = &limitedVideoLog{}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("video conversion: %v: %s", err, cmd.Stderr)
		return errInvalidVideo
	}
	info, err := os.Stat(target)
	if err != nil || info.Size() == 0 {
		return errInvalidVideo
	}
	return nil
}

type limitedVideoLog struct{ data []byte }

func (b *limitedVideoLog) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 4096 - len(b.data)
	if remaining > 0 {
		b.data = append(b.data, p[:min(remaining, n)]...)
	}
	return n, nil
}
func (b *limitedVideoLog) String() string { return string(b.data) }

func writeVideoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errVideoTools):
		log.Print(err)
		http.Error(w, "Video uploads require FFmpeg and ffprobe on the server. Images can still be uploaded.", http.StatusServiceUnavailable)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		http.Error(w, "Video conversion timed out or was cancelled. Try a shorter clip.", http.StatusRequestTimeout)
	default:
		http.Error(w, "This file could not be converted. Choose a valid, unencrypted video in a supported format.", http.StatusBadRequest)
	}
}
