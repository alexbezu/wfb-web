//go:build gstreamer

package rtsp

/*
#cgo pkg-config: gstreamer-rtsp-server-1.0
#include <gst/gst.h>
#include <gst/rtsp-server/rtsp-server.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

static GMainLoop *wfbweb_rtsp_loop = NULL;

static gboolean wfbweb_rtsp_cleanup_sessions(GstRTSPServer *server) {
	GstRTSPSessionPool *pool = gst_rtsp_server_get_session_pool(server);
	gst_rtsp_session_pool_cleanup(pool);
	g_object_unref(pool);
	return TRUE;
}

static int wfbweb_rtsp_run(int mode, int mtu, int latency, char *uri, char *rtsp_port, int rtp_port) {
	GMainLoop *loop;
	GstRTSPServer *server;
	GstRTSPMountPoints *mounts;
	GstRTSPMediaFactory *factory;
	char pipeline[2048];

	gst_init(NULL, NULL);

	loop = g_main_loop_new(NULL, FALSE);
	wfbweb_rtsp_loop = loop;

	server = gst_rtsp_server_new();
	gst_rtsp_server_set_service(server, rtsp_port);

	mounts = gst_rtsp_server_get_mount_points(server);
	factory = gst_rtsp_media_factory_new();

	snprintf(pipeline, sizeof(pipeline),
		"( udpsrc port=%d ! application/x-rtp,media=video,clock-rate=90000,encoding-name=H%d ! rtpjitterbuffer latency=%d ! rtph%ddepay ! rtph%dpay name=pay0 pt=96 config-interval=1 aggregate-mode=zero-latency mtu=%d )",
		rtp_port, mode, latency, mode, mode, mtu);

	gst_rtsp_media_factory_set_launch(factory, pipeline);
	gst_rtsp_media_factory_set_shared(factory, TRUE);
	gst_rtsp_mount_points_add_factory(mounts, uri, factory);
	g_object_unref(mounts);

	if (gst_rtsp_server_attach(server, NULL) == 0) {
		wfbweb_rtsp_loop = NULL;
		g_main_loop_unref(loop);
		g_object_unref(server);
		return -1;
	}

	g_timeout_add_seconds(2, (GSourceFunc) wfbweb_rtsp_cleanup_sessions, server);
	g_main_loop_run(loop);

	wfbweb_rtsp_loop = NULL;
	g_main_loop_unref(loop);
	g_object_unref(server);
	return 0;
}

static void wfbweb_rtsp_stop(void) {
	if (wfbweb_rtsp_loop != NULL) {
		g_main_loop_quit(wfbweb_rtsp_loop);
	}
}
*/
import "C"

import (
	"errors"
	"strconv"
	"unsafe"
)

func nativeAvailable() bool {
	return true
}

func runNative(opts Options) error {
	mode := 265
	if opts.Codec == "h264" {
		mode = 264
	}

	uri := C.CString(opts.URI)
	port := C.CString(strconv.Itoa(opts.Port))
	defer C.free(unsafe.Pointer(uri))
	defer C.free(unsafe.Pointer(port))

	if rc := C.wfbweb_rtsp_run(C.int(mode), C.int(opts.MTU), C.int(opts.Latency), uri, port, C.int(opts.RTPPort)); rc != 0 {
		return errors.New("failed to attach GStreamer RTSP server")
	}
	return nil
}

func stopNative() {
	C.wfbweb_rtsp_stop()
}
