//go:build !gstreamer

package rtsp

import "errors"

func nativeAvailable() bool {
	return false
}

func runNative(Options) error {
	return errors.New("native GStreamer RTSP support is not built")
}

func stopNative() {}
