.PHONY: mac linux build run run-debug clean help gst-encoders \
	audio-mac-opus audio-mac-pcmu audio-linux-opus audio-linux-pcmu \
	video-mac-h264 video-mac-h265 video-mac-vp8 video-mac-vp9 video-mac-av1 \
	video-linux-h264 video-linux-h265 video-linux-vp8 video-linux-vp9 video-linux-av1 \
	video-raspi-h264 video-jetson-h264 video-jetson-h265 video-jetson-vp8 video-jetson-vp9 \
	video-rock5-h264 video-rock5-h265 video-rock5-vp8 video-rock5-vp9 video-rock5-av1

VIDEO_HOST ?= 127.0.0.1
AUDIO_HOST ?= 127.0.0.1
VIDEO_PORT ?= 5000
AUDIO_PORT ?= 5001
VIDEO_WIDTH ?= 1280
VIDEO_HEIGHT ?= 720
VIDEO_FPS ?= 30/1

mac: build

linux: build

build:
	go build -x -v ./...

run:
	./cli

run-debug:
	GST_DEBUG=3 ./cli

clean:
	rm -f cli

gst-encoders:
	gst-inspect-1.0 | grep -Ei 'enc' | grep -Ei '264|265|av1|vp8|vp9|opus|mulaw' | grep -Eiv 'json|timestamper|alpha'

audio-mac-opus:
	GST_DEBUG=2 gst-launch-1.0 -v -e osxaudiosrc do-timestamp=true ! audio/x-raw,rate=48000,channels=2 ! queue max-size-buffers=1 leaky=downstream ! audioconvert ! audioresample ! opusenc ! rtpopuspay ! udpsink host=$(AUDIO_HOST) port=$(AUDIO_PORT) sync=false async=false

audio-mac-pcmu:
	GST_DEBUG=2 gst-launch-1.0 -v -e osxaudiosrc do-timestamp=true ! audio/x-raw,rate=48000,channels=2 ! queue max-size-buffers=1 leaky=downstream ! audioconvert ! audioresample ! mulawenc ! rtppcmupay ! udpsink host=$(AUDIO_HOST) port=$(AUDIO_PORT) sync=false async=false

audio-linux-opus:
	GST_DEBUG=2 gst-launch-1.0 -v -e pipewiresrc do-timestamp=true ! audio/x-raw,rate=48000,channels=2 ! audioconvert ! audioresample ! queue max-size-buffers=1 leaky=downstream ! opusenc ! rtpopuspay ! udpsink host=$(AUDIO_HOST) port=$(AUDIO_PORT) sync=false async=false

audio-linux-pcmu:
	GST_DEBUG=2 gst-launch-1.0 -v -e pipewiresrc do-timestamp=true ! audio/x-raw,rate=48000,channels=2 ! audioconvert ! audioresample ! queue max-size-buffers=1 leaky=downstream ! mulawenc ! rtppcmupay ! udpsink host=$(AUDIO_HOST) port=$(AUDIO_PORT) sync=false async=false

video-mac-h264:
	GST_DEBUG=2 gst-launch-1.0 -v -e avfvideosrc do-stats=true do-timestamp=true ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS),format=NV12 ! queue max-size-buffers=1 leaky=downstream ! vtenc_h264_hw realtime=true ! h264parse ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-mac-h265:
	GST_DEBUG=2 gst-launch-1.0 -v -e avfvideosrc do-stats=true do-timestamp=true ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS),format=NV12 ! queue max-size-buffers=1 leaky=downstream ! vtenc_h265_hw realtime=true allow-frame-reordering=false ! h265parse ! rtph265pay config-interval=-1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-mac-vp8:
	GST_DEBUG=2 gst-launch-1.0 -v -e avfvideosrc do-stats=true do-timestamp=true ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! vp8enc deadline=1 ! rtpvp8pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-mac-vp9:
	GST_DEBUG=2 gst-launch-1.0 -v -e avfvideosrc do-stats=true do-timestamp=true ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! vp9enc deadline=1 cpu-used=8 threads=4 lag-in-frames=0 ! vp9parse ! rtpvp9pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-mac-av1:
	GST_DEBUG=2 gst-launch-1.0 -v -e avfvideosrc do-stats=true do-timestamp=true ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! svtav1enc ! av1parse ! rtpav1pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-linux-h264:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! vaapipostproc ! video/x-raw(memory:VASurface),format=NV12,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! queue max-size-buffers=1 leaky=downstream ! vaapih264enc ! h264parse ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-linux-h265:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! vaapipostproc ! video/x-raw(memory:VASurface),format=NV12,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! queue max-size-buffers=1 leaky=downstream ! vaapih265enc ! h265parse ! rtph265pay config-interval=-1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-linux-vp8:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! vp8enc deadline=1 ! rtpvp8pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-linux-vp9:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! vp9enc deadline=1 cpu-used=4 ! vp9parse ! rtpvp9pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-linux-av1:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! svtav1enc ! av1parse ! rtpav1pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-raspi-h264:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS),format=NV12 ! queue max-size-buffers=1 leaky=downstream ! v4l2h264enc capture-io-mode=dmabuf output-io-mode=dmabuf ! capsfilter caps=video/x-h264,level=(string)4.1 ! queue max-size-buffers=3 leaky=downstream ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-jetson-h264:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src ! video/x-raw(memory:NVMM),width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! queue max-size-buffers=1 leaky=downstream ! nvv4l2h264enc preset-level=3 profile=4 bitrate=20000000 ! capsfilter caps=video/x-h264,level=(string)4 ! queue max-size-buffers=3 leaky=downstream ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-jetson-h265:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src ! video/x-raw(memory:NVMM),width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! queue max-size-buffers=1 leaky=downstream ! nvv4l2h265enc preset-level=3 profile=0 bitrate=30000000 ! capsfilter caps=video/x-h265,level=(string)4 ! queue max-size-buffers=3 leaky=downstream ! rtph265pay config-interval=1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-jetson-vp8:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src ! video/x-raw(memory:NVMM),width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! queue max-size-buffers=1 leaky=downstream ! nvv4l2vp8enc bitrate=20000000 ! rtpvp8pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-jetson-vp9:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src ! video/x-raw(memory:NVMM),width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! queue max-size-buffers=1 leaky=downstream ! nvv4l2vp9enc bitrate=30000000 ! rtpvp9pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-rock5-h264:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS),format=YUY2 ! videoconvert ! video/x-raw,format=NV12 ! queue max-size-buffers=1 leaky=downstream ! mpph264enc level=40 profile=100 ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-rock5-h265:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS),format=YUY2 ! videoconvert ! video/x-raw,format=NV12 ! queue max-size-buffers=1 leaky=downstream ! mpph265enc ! rtph265pay config-interval=1 aggregate-mode=zero-latency ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-rock5-vp8:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS),format=YUY2 ! videoconvert ! video/x-raw,format=NV12 ! queue max-size-buffers=1 leaky=downstream ! mppvp8enc ! rtpvp8pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-rock5-vp9:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! vp9enc deadline=1 cpu-used=4 ! vp9parse ! rtpvp9pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

video-rock5-av1:
	GST_DEBUG=2 gst-launch-1.0 -v -e v4l2src do-timestamp=true io-mode=dmabuf ! video/x-raw,width=$(VIDEO_WIDTH),height=$(VIDEO_HEIGHT),framerate=$(VIDEO_FPS) ! videoconvert ! video/x-raw,format=I420 ! queue max-size-buffers=1 leaky=downstream ! svtav1enc ! av1parse ! rtpav1pay ! udpsink host=$(VIDEO_HOST) port=$(VIDEO_PORT) sync=false async=false

help:
	@echo "Targets:"
	@echo "  mac       Build cli (alias of build)"
	@echo "  linux     Build cli (alias of build)"
	@echo "  build     go build -o cli"
	@echo "  run       Run ./cli"
	@echo "  run-debug Run ./cli with GST_DEBUG=3"
	@echo "  clean     Remove ./cli"
	@echo "  gst-encoders Filter gst-inspect for encoders (264/265/av1/vp8/vp9/opus/mulaw)"
	@echo "  audio-mac-opus  RTP OPUS audio sender on macOS (port $(AUDIO_PORT))"
	@echo "  audio-mac-pcmu  RTP PCMU audio sender on macOS (port $(AUDIO_PORT))"
	@echo "  audio-linux-opus RTP OPUS audio sender on Linux (port $(AUDIO_PORT))"
	@echo "  audio-linux-pcmu RTP PCMU audio sender on Linux (port $(AUDIO_PORT))"
	@echo "  video-mac-h264 RTP H.264 video sender on macOS (port $(VIDEO_PORT))"
	@echo "  video-mac-h265 RTP H.265 video sender on macOS (port $(VIDEO_PORT))"
	@echo "  video-mac-vp8  RTP VP8 video sender on macOS (port $(VIDEO_PORT))"
	@echo "  video-mac-vp9  RTP VP9 video sender on macOS (port $(VIDEO_PORT))"
	@echo "  video-mac-av1  RTP AV1 video sender on macOS (port $(VIDEO_PORT))"
	@echo "  video-linux-h264 RTP H.264 video sender on Linux (port $(VIDEO_PORT))"
	@echo "  video-linux-h265 RTP H.265 video sender on Linux (port $(VIDEO_PORT))"
	@echo "  video-linux-vp8  RTP VP8 video sender on Linux (port $(VIDEO_PORT))"
	@echo "  video-linux-vp9  RTP VP9 video sender on Linux (port $(VIDEO_PORT))"
	@echo "  video-linux-av1  RTP AV1 video sender on Linux (port $(VIDEO_PORT))"
	@echo "  video-raspi-h264 RTP H.264 video sender on Raspberry Pi (port $(VIDEO_PORT))"
	@echo "  video-jetson-h264 RTP H.264 video sender on Jetson (port $(VIDEO_PORT))"
	@echo "  video-jetson-h265 RTP H.265 video sender on Jetson (port $(VIDEO_PORT))"
	@echo "  video-jetson-vp8  RTP VP8 video sender on Jetson (port $(VIDEO_PORT))"
	@echo "  video-jetson-vp9  RTP VP9 video sender on Jetson (port $(VIDEO_PORT))"
	@echo "  video-rock5-h264 RTP H.264 video sender on Rock5 (port $(VIDEO_PORT))"
	@echo "  video-rock5-h265 RTP H.265 video sender on Rock5 (port $(VIDEO_PORT))"
	@echo "  video-rock5-vp8  RTP VP8 video sender on Rock5 (port $(VIDEO_PORT))"
	@echo "  video-rock5-vp9  RTP VP9 video sender on Rock5 (port $(VIDEO_PORT))"
	@echo "  video-rock5-av1  RTP AV1 video sender on Rock5 (port $(VIDEO_PORT))"
