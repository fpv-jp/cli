// This is a simplified go-reimplementation of the gst-launch-<version> cli tool.
// It builds a pipeline from interactive prompts instead of CLI pipeline strings.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/examples"
	"github.com/go-gst/go-gst/gst"
)

type Mode struct {
	Format    string
	Width     int
	Height    int
	Framerate string
}

type Codec string

const (
	CodecH264 Codec = "H264"
	CodecH265 Codec = "H265"
	CodecVP8  Codec = "VP8"
	CodecVP9  Codec = "VP9"
	CodecAV1  Codec = "AV1"
)

type AudioCodec string

const (
	AudioOpus AudioCodec = "OPUS"
	AudioPCMU AudioCodec = "PCMU"
)

type LinuxH264Mode string

const (
	LinuxH264VAAPI      LinuxH264Mode = "vaapi"
	LinuxH264RaspiV4L2  LinuxH264Mode = "raspi-v4l2"
	LinuxH264Libcamera  LinuxH264Mode = "libcamera"
	LinuxH264CameraH264 LinuxH264Mode = "camera-h264"
)

type LinuxVariant string

const (
	LinuxGeneric LinuxVariant = "generic"
	LinuxRaspi   LinuxVariant = "raspi"
	LinuxJetson  LinuxVariant = "jetson"
	LinuxRock5   LinuxVariant = "rock5"
)

func (m Mode) String() string {
	return fmt.Sprintf("%dx%d %s %s", m.Width, m.Height, m.Framerate, m.Format)
}

func runPipeline(mainLoop *glib.MainLoop) error {
	if len(os.Args) > 1 {
		fmt.Fprintln(os.Stderr, "ignoring CLI arguments; this tool is interactive")
	}

	gst.Init(nil)

	reader := bufio.NewReader(os.Stdin)

	platform, err := detectPlatform()
	if err != nil {
		return err
	}
	linuxVariant := LinuxGeneric
	if platform == "linux" {
		linuxVariant = detectLinuxVariant()
	}

	host, err := promptString(reader, "Video UDP host", "127.0.0.1")
	if err != nil {
		return err
	}
	port, err := promptPort(reader, "Video UDP port", 5000)
	if err != nil {
		return err
	}
	codec, err := promptCodec(reader, platform, linuxVariant)
	if err != nil {
		return err
	}
	linuxH264Mode := LinuxH264VAAPI
	if platform == "linux" && codec == CodecH264 && linuxVariant != LinuxJetson {
		linuxH264Mode, err = promptLinuxH264Mode(reader, linuxVariant)
		if err != nil {
			return err
		}
	}

	videoDevice, err := selectDevice(reader, "Video/Source", "video/x-raw", "Select a camera")
	if err != nil {
		return err
	}
	mode, err := pickMode(reader, videoDevice.GetCaps())
	if err != nil {
		return err
	}

	audioHost, err := promptString(reader, "Audio UDP host", host)
	if err != nil {
		return err
	}
	audioPort, err := promptPort(reader, "Audio UDP port", 5001)
	if err != nil {
		return err
	}
	audioCodec, err := promptAudioCodec(reader)
	if err != nil {
		return err
	}
	audioDevice, err := selectDevice(reader, "Audio/Source", "audio/x-raw", "Select an audio device")
	if err != nil {
		return err
	}

	sourceName := "avfvideosrc"
	if platform == "linux" {
		sourceName = "v4l2src"
	}
	if elem := videoDevice.CreateElement(""); elem != nil {
		if factory := elem.GetFactory(); factory != nil && factory.GetName() != "" {
			sourceName = factory.GetName()
		}
	}

	videoDeviceProp := buildDeviceProperty(videoDevice)
	videoPipelineStr := buildVideoPipelineString(platform, linuxVariant, sourceName, videoDeviceProp, mode, host, port, codec, linuxH264Mode)

	audioSourceName := "osxaudiosrc"
	if platform == "linux" {
		audioSourceName = "pipewiresrc"
	}
	if elem := audioDevice.CreateElement(""); elem != nil {
		if factory := elem.GetFactory(); factory != nil && factory.GetName() != "" {
			audioSourceName = factory.GetName()
		}
	}
	audioDeviceProp := buildDeviceProperty(audioDevice)
	audioPipelineStr := buildAudioPipelineString(platform, audioSourceName, audioDeviceProp, audioHost, audioPort, audioCodec)

	// Let GStreamer create a pipeline from the selected parameters.
	videoPipeline, err := gst.NewPipelineFromString(videoPipelineStr)
	if err != nil {
		return err
	}
	audioPipeline, err := gst.NewPipelineFromString(audioPipelineStr)
	if err != nil {
		return err
	}

	allPipelines := []*gst.Pipeline{videoPipeline, audioPipeline}
	addPipelineWatch(videoPipeline, "video", mainLoop, allPipelines)
	addPipelineWatch(audioPipeline, "audio", mainLoop, allPipelines)

	// Start the pipelines
	videoPipeline.SetState(gst.StatePlaying)
	audioPipeline.SetState(gst.StatePlaying)

	// Block on the main loop
	return mainLoop.RunError()
}

func buildDeviceProperty(device *gst.Device) string {
	if device != nil {
		if props := device.GetProperties(); props != nil {
			values := props.Values()
			if s := stringProp(values, "device"); s != "" {
				return fmt.Sprintf("device=%s", strconv.Quote(s))
			}
			if s := stringProp(values, "path"); s != "" {
				return fmt.Sprintf("device=%s", strconv.Quote(s))
			}
			if s := stringProp(values, "target-object"); s != "" {
				return fmt.Sprintf("target-object=%s", strconv.Quote(s))
			}
			if s := stringProp(values, "node.id"); s != "" {
				return fmt.Sprintf("target-object=%s", strconv.Quote(s))
			}
			if i := intProp(values, "device-index"); i >= 0 {
				return fmt.Sprintf("device-index=%d", i)
			}
		}
	}
	return ""
}

func buildVideoPipelineString(platform string, linuxVariant LinuxVariant, sourceName, deviceProp string, mode Mode, host string, port int, codec Codec, linuxH264Mode LinuxH264Mode) string {
	devicePrefix := devicePropPrefix(deviceProp)
	switch platform {
	case "linux":
		return buildLinuxVideoPipelineString(linuxVariant, sourceName, devicePrefix, mode, host, port, codec, linuxH264Mode)
	default:
		return buildDarwinVideoPipelineString(sourceName, devicePrefix, mode, host, port, codec)
	}
}

func buildDarwinVideoPipelineString(sourceName, devicePrefix string, mode Mode, host string, port int, codec Codec) string {
	caps := fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%s,format=%s",
		mode.Width, mode.Height, mode.Framerate, mode.Format)
	switch codec {
	case CodecH265:
		return fmt.Sprintf(
			"%s do-stats=true do-timestamp=true %s! %s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vtenc_h265_hw realtime=true allow-frame-reordering=false ! "+
				"h265parse ! rtph265pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, caps, host, port,
		)
	case CodecVP8:
		return fmt.Sprintf(
			"%s do-stats=true do-timestamp=true %s! video/x-raw,width=%d,height=%d,framerate=%s ! "+
				"videoconvert ! video/x-raw,format=I420 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vp8enc deadline=1 ! "+
				"rtpvp8pay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecVP9:
		return fmt.Sprintf(
			"%s do-stats=true do-timestamp=true %s! video/x-raw,width=%d,height=%d,framerate=%s ! "+
				"videoconvert ! video/x-raw,format=I420 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vp9enc deadline=1 cpu-used=8 threads=4 lag-in-frames=0 ! "+
				"vp9parse ! rtpvp9pay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecAV1:
		return fmt.Sprintf(
			"%s do-stats=true do-timestamp=true %s! video/x-raw,width=%d,height=%d,framerate=%s ! "+
				"videoconvert ! video/x-raw,format=I420 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"svtav1enc ! "+
				"av1parse ! rtpav1pay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	default:
		return fmt.Sprintf(
			"%s do-stats=true do-timestamp=true %s! %s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vtenc_h264_hw realtime=true ! "+
				"h264parse ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, caps, host, port,
		)
	}
}

func buildLinuxVideoPipelineString(linuxVariant LinuxVariant, sourceName, devicePrefix string, mode Mode, host string, port int, codec Codec, linuxH264Mode LinuxH264Mode) string {
	if linuxVariant == LinuxJetson {
		return buildJetsonVideoPipelineString(sourceName, devicePrefix, mode, host, port, codec)
	}
	if linuxVariant == LinuxRock5 {
		return buildRock5VideoPipelineString(sourceName, devicePrefix, mode, host, port, codec)
	}
	if codec == CodecH264 {
		return buildLinuxH264PipelineString(sourceName, devicePrefix, mode, host, port, linuxH264Mode)
	}
	switch codec {
	case CodecH265:
		return fmt.Sprintf(
			"%s do-timestamp=true %sio-mode=dmabuf ! vaapipostproc ! "+
				"video/x-raw(memory:VASurface),format=NV12,width=%d,height=%d,framerate=%s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vaapih265enc ! h265parse ! rtph265pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecVP8:
		return fmt.Sprintf(
			"%s do-timestamp=true %sio-mode=dmabuf ! "+
				"video/x-raw,width=%d,height=%d,framerate=%s ! "+
				"videoconvert ! video/x-raw,format=I420 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vp8enc deadline=1 ! rtpvp8pay ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecVP9:
		return fmt.Sprintf(
			"%s do-timestamp=true %sio-mode=dmabuf ! "+
				"video/x-raw,width=%d,height=%d,framerate=%s ! "+
				"videoconvert ! video/x-raw,format=I420 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vp9enc deadline=1 cpu-used=4 ! vp9parse ! rtpvp9pay ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecAV1:
		return fmt.Sprintf(
			"%s do-timestamp=true %sio-mode=dmabuf ! "+
				"video/x-raw,width=%d,height=%d,framerate=%s ! "+
				"videoconvert ! video/x-raw,format=I420 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"svtav1enc ! av1parse ! rtpav1pay ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	default:
		return buildLinuxH264PipelineString(sourceName, devicePrefix, mode, host, port, linuxH264Mode)
	}
}

func buildRock5VideoPipelineString(sourceName, devicePrefix string, mode Mode, host string, port int, codec Codec) string {
	switch codec {
	case CodecH265:
		return fmt.Sprintf(
			"%s %s! video/x-raw,width=%d,height=%d,framerate=%s,format=YUY2 ! "+
				"videoconvert ! video/x-raw,format=NV12 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"mpph265enc ! "+
				"rtph265pay config-interval=1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecVP8:
		return fmt.Sprintf(
			"%s %s! video/x-raw,width=%d,height=%d,framerate=%s,format=YUY2 ! "+
				"videoconvert ! video/x-raw,format=NV12 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"mppvp8enc ! "+
				"rtpvp8pay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecVP9:
		fallthrough
	case CodecAV1:
		return buildLinuxVideoPipelineString(LinuxGeneric, sourceName, devicePrefix, mode, host, port, codec, LinuxH264VAAPI)
	default:
		return fmt.Sprintf(
			"%s %s! video/x-raw,width=%d,height=%d,framerate=%s,format=YUY2 ! "+
				"videoconvert ! video/x-raw,format=NV12 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"mpph264enc level=40 profile=100 ! "+
				"rtph264pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	}
}

func buildJetsonVideoPipelineString(sourceName, devicePrefix string, mode Mode, host string, port int, codec Codec) string {
	switch codec {
	case CodecH265:
		return fmt.Sprintf(
			"%s %s! video/x-raw(memory:NVMM),width=%d,height=%d,framerate=%s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"nvv4l2h265enc preset-level=3 profile=0 bitrate=30000000 ! "+
				"capsfilter caps=video/x-h265,level=(string)4 ! "+
				"queue max-size-buffers=3 leaky=downstream ! "+
				"rtph265pay config-interval=1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecVP8:
		return fmt.Sprintf(
			"%s %s! video/x-raw(memory:NVMM),width=%d,height=%d,framerate=%s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"nvv4l2vp8enc bitrate=20000000 ! "+
				"rtpvp8pay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case CodecVP9:
		return fmt.Sprintf(
			"%s %s! video/x-raw(memory:NVMM),width=%d,height=%d,framerate=%s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"nvv4l2vp9enc bitrate=30000000 ! "+
				"rtpvp9pay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	default:
		return fmt.Sprintf(
			"%s %s! video/x-raw(memory:NVMM),width=%d,height=%d,framerate=%s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"nvv4l2h264enc preset-level=3 profile=4 bitrate=20000000 ! "+
				"capsfilter caps=video/x-h264,level=(string)4 ! "+
				"queue max-size-buffers=3 leaky=downstream ! "+
				"rtph264pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	}
}

func buildLinuxH264PipelineString(sourceName, devicePrefix string, mode Mode, host string, port int, linuxH264Mode LinuxH264Mode) string {
	switch linuxH264Mode {
	case LinuxH264RaspiV4L2:
		return fmt.Sprintf(
			"%s do-timestamp=true %sio-mode=dmabuf ! "+
				"video/x-raw,width=%d,height=%d,framerate=%s,format=NV12 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"v4l2h264enc capture-io-mode=dmabuf output-io-mode=dmabuf ! "+
				"capsfilter caps=video/x-h264,level=(string)4.1 ! "+
				"queue max-size-buffers=3 leaky=downstream ! "+
				"rtph264pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case LinuxH264Libcamera:
		return fmt.Sprintf(
			"libcamerasrc %s! video/x-raw,width=%d,height=%d,framerate=%s,format=YUY2,interlace-mode=progressive ! "+
				"v4l2h264enc extra-controls=\"encode,h264_profile=4,h264_level=12,video_bitrate=20000000\" ! "+
				"capsfilter caps=video/x-h264,level=(string)4.1 ! "+
				"queue max-size-buffers=3 leaky=downstream ! "+
				"rtph264pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	case LinuxH264CameraH264:
		return fmt.Sprintf(
			"%s do-timestamp=true %s! video/x-h264,width=%d,height=%d,framerate=%s,stream-format=byte-stream ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"h264parse ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	default:
		return fmt.Sprintf(
			"%s do-timestamp=true %sio-mode=dmabuf ! vaapipostproc ! "+
				"video/x-raw(memory:VASurface),format=NV12,width=%d,height=%d,framerate=%s ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"vaapih264enc ! h264parse ! rtph264pay config-interval=-1 aggregate-mode=zero-latency ! "+
				"udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, mode.Width, mode.Height, mode.Framerate, host, port,
		)
	}
}

func buildAudioPipelineString(platform string, sourceName, deviceProp string, host string, port int, codec AudioCodec) string {
	devicePrefix := devicePropPrefix(deviceProp)
	if platform == "linux" {
		return buildLinuxAudioPipelineString(sourceName, devicePrefix, host, port, codec)
	}
	return buildDarwinAudioPipelineString(sourceName, devicePrefix, host, port, codec)
}

func buildDarwinAudioPipelineString(sourceName, devicePrefix string, host string, port int, codec AudioCodec) string {
	switch codec {
	case AudioPCMU:
		return fmt.Sprintf(
			"%s %sdo-timestamp=true ! audio/x-raw,rate=48000,channels=2 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"audioconvert ! audioresample ! mulawenc ! "+
				"rtppcmupay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, host, port,
		)
	default:
		return fmt.Sprintf(
			"%s %sdo-timestamp=true ! audio/x-raw,rate=48000,channels=2 ! "+
				"queue max-size-buffers=1 leaky=downstream ! "+
				"audioconvert ! audioresample ! opusenc ! "+
				"rtpopuspay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, host, port,
		)
	}
}

func buildLinuxAudioPipelineString(sourceName, devicePrefix string, host string, port int, codec AudioCodec) string {
	switch codec {
	case AudioPCMU:
		return fmt.Sprintf(
			"%s do-timestamp=true %s! audio/x-raw,rate=48000,channels=2 ! "+
				"audioconvert ! audioresample ! queue max-size-buffers=1 leaky=downstream ! "+
				"mulawenc ! rtppcmupay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, host, port,
		)
	default:
		return fmt.Sprintf(
			"%s do-timestamp=true %s! audio/x-raw,rate=48000,channels=2 ! "+
				"audioconvert ! audioresample ! queue max-size-buffers=1 leaky=downstream ! "+
				"opusenc ! rtpopuspay ! udpsink host=%s port=%d sync=false async=false",
			sourceName, devicePrefix, host, port,
		)
	}
}

func pickMode(reader *bufio.Reader, caps *gst.Caps) (Mode, error) {
	resolutions := []Mode{
		{Width: 640, Height: 480},
		{Width: 1024, Height: 768},
		{Width: 1440, Height: 1080},
		{Width: 1280, Height: 720},
		{Width: 1920, Height: 1080},
		{Width: 3840, Height: 2160},
		{Width: 0, Height: 0},
	}
	resLabels := []string{
		"640x480 (VGA 4:3)",
		"1024x768 (XGA 4:3)",
		"1440x1080 (HD 4:3)",
		"1280x720 (HDTV 16:9)",
		"1920x1080 (2K/FHD 16:9)",
		"3840x2160 (4K/UHD 16:9)",
		"Custom (enter width/height)",
	}
	idx, err := promptChoice(reader, "Select a resolution", resLabels)
	if err != nil {
		return Mode{}, err
	}

	width := resolutions[idx].Width
	height := resolutions[idx].Height
	if width == 0 && height == 0 {
		width, err = promptInt(reader, "Width", 640)
		if err != nil {
			return Mode{}, err
		}
		height, err = promptInt(reader, "Height", 480)
		if err != nil {
			return Mode{}, err
		}
	}

	format, err := promptFormat(reader)
	if err != nil {
		return Mode{}, err
	}
	fps, err := promptFraction(reader, "Framerate (num/den)", "30/1")
	if err != nil {
		return Mode{}, err
	}
	if caps != nil && !caps.IsEmpty() {
		fmt.Println("Note: chosen resolution may not be supported by the device caps.")
	}
	return Mode{
		Format:    format,
		Width:     width,
		Height:    height,
		Framerate: fps,
	}, nil
}

func extractModes(caps *gst.Caps, limit int) []Mode {
	if caps == nil || caps.IsEmpty() {
		return nil
	}
	modes := []Mode{}
	seen := make(map[string]struct{})
	for i := 0; i < caps.GetSize(); i++ {
		st := caps.GetStructureAt(i)
		if st == nil || st.Name() != "video/x-raw" {
			continue
		}
		formats := extractStringValues(getStructureValue(st, "format"))
		widths := extractIntValues(getStructureValue(st, "width"))
		heights := extractIntValues(getStructureValue(st, "height"))
		fps := extractFractionValues(getStructureValue(st, "framerate"))
		if len(formats) == 0 || len(widths) == 0 || len(heights) == 0 || len(fps) == 0 {
			continue
		}
		for _, format := range formats {
			for _, w := range widths {
				for _, h := range heights {
					for _, fr := range fps {
						key := fmt.Sprintf("%s|%d|%d|%s", format, w, h, fr)
						if _, ok := seen[key]; ok {
							continue
						}
						seen[key] = struct{}{}
						modes = append(modes, Mode{
							Format:    format,
							Width:     w,
							Height:    h,
							Framerate: fr,
						})
						if len(modes) >= limit {
							return modes
						}
					}
				}
			}
		}
	}
	return modes
}

func getStructureValue(st *gst.Structure, key string) any {
	if st == nil {
		return nil
	}
	val, err := st.GetValue(key)
	if err != nil {
		return nil
	}
	return val
}

func extractStringValues(val any) []string {
	switch v := val.(type) {
	case string:
		return []string{v}
	case *gst.ValueListValue:
		return extractFromValueList(v, extractStringValues)
	case *gst.ValueArrayValue:
		return extractFromValueArray(v, extractStringValues)
	default:
		return nil
	}
}

func extractIntValues(val any) []int {
	switch v := val.(type) {
	case int:
		return []int{v}
	case int32:
		return []int{int(v)}
	case int64:
		return []int{int(v)}
	case uint:
		return []int{int(v)}
	case uint32:
		return []int{int(v)}
	case uint64:
		return []int{int(v)}
	case *gst.ValueListValue:
		return extractFromValueListInt(v, extractIntValues)
	case *gst.ValueArrayValue:
		return extractFromValueArrayInt(v, extractIntValues)
	default:
		return nil
	}
}

func extractFractionValues(val any) []string {
	switch v := val.(type) {
	case *gst.FractionValue:
		return []string{v.String()}
	case string:
		if _, err := parseFraction(v); err == nil {
			return []string{v}
		}
	case *gst.ValueListValue:
		return extractFromValueList(v, extractFractionValues)
	case *gst.ValueArrayValue:
		return extractFromValueArray(v, extractFractionValues)
	default:
		return nil
	}
	return nil
}

func extractFromValueList(list *gst.ValueListValue, fn func(any) []string) []string {
	if list == nil {
		return nil
	}
	var out []string
	for i := uint(0); i < list.Size(); i++ {
		out = append(out, fn(list.ValueAt(i))...)
	}
	return out
}

func extractFromValueArray(list *gst.ValueArrayValue, fn func(any) []string) []string {
	if list == nil {
		return nil
	}
	var out []string
	for i := uint(0); i < list.Size(); i++ {
		out = append(out, fn(list.ValueAt(i))...)
	}
	return out
}

func extractFromValueListInt(list *gst.ValueListValue, fn func(any) []int) []int {
	if list == nil {
		return nil
	}
	var out []int
	for i := uint(0); i < list.Size(); i++ {
		out = append(out, fn(list.ValueAt(i))...)
	}
	return out
}

func extractFromValueArrayInt(list *gst.ValueArrayValue, fn func(any) []int) []int {
	if list == nil {
		return nil
	}
	var out []int
	for i := uint(0); i < list.Size(); i++ {
		out = append(out, fn(list.ValueAt(i))...)
	}
	return out
}

func promptChoice(reader *bufio.Reader, prompt string, options []string) (int, error) {
	if len(options) == 0 {
		return 0, errors.New("no options available")
	}
	for {
		fmt.Println(prompt + ":")
		for i, opt := range options {
			fmt.Printf("  %d) %s\n", i+1, opt)
		}
		fmt.Printf("Select 1-%d [1]: ", len(options))
		line, err := readLine(reader)
		if err != nil {
			return 0, err
		}
		if line == "" {
			return 0, nil
		}
		n, err := parseSelection(line)
		if err != nil || n < 1 || n > len(options) {
			fmt.Println("Invalid selection.")
			continue
		}
		return n - 1, nil
	}
}

func selectDevice(reader *bufio.Reader, className, capsStr, prompt string) (*gst.Device, error) {
	monitor := gst.NewDeviceMonitor()
	filterCaps := gst.NewCapsFromString(capsStr)
	monitor.AddFilter(className, filterCaps)
	monitor.Start()
	devices := monitor.GetDevices()
	monitor.Stop()
	if len(devices) == 0 {
		return nil, fmt.Errorf("no devices found for %s", className)
	}

	deviceNames := make([]string, 0, len(devices))
	for _, d := range devices {
		deviceNames = append(deviceNames, d.GetDisplayName())
	}
	idx, err := promptChoice(reader, prompt, deviceNames)
	if err != nil {
		return nil, err
	}
	return devices[idx], nil
}

func promptCodec(reader *bufio.Reader, platform string, linuxVariant LinuxVariant) (Codec, error) {
	var options []string
	if platform == "linux" {
		h264Label := "H264"
		h265Label := "H265"
		switch linuxVariant {
		case LinuxJetson:
			h264Label = "H264 (nvv4l2h264enc)"
			h265Label = "H265 (nvv4l2h265enc)"
		case LinuxRock5:
			h264Label = "H264 (mpph264enc)"
			h265Label = "H265 (mpph265enc)"
		default:
			h264Label = "H264 (vaapih264enc or variant-specific)"
			h265Label = "H265 (vaapih265enc)"
		}
		options = []string{
			h264Label,
			h265Label,
			"VP8 (vp8enc)",
			"VP9 (vp9enc)",
			"AV1 (svtav1enc)",
		}
	} else {
		options = []string{
			"H264 (vtenc_h264_hw)",
			"H265 (vtenc_h265_hw)",
			"VP8 (vp8enc)",
			"VP9 (vp9enc)",
			"AV1 (svtav1enc)",
		}
	}
	idx, err := promptChoice(reader, "Select a codec", options)
	if err != nil {
		return CodecH264, err
	}
	switch idx {
	case 1:
		return CodecH265, nil
	case 2:
		return CodecVP8, nil
	case 3:
		return CodecVP9, nil
	case 4:
		return CodecAV1, nil
	default:
		return CodecH264, nil
	}
}

func promptLinuxH264Mode(reader *bufio.Reader, variant LinuxVariant) (LinuxH264Mode, error) {
	raspi := variant == LinuxRaspi
	options := []string{
		"VAAPI (vaapipostproc + vaapih264enc)",
		"Raspberry Pi v4l2h264enc",
		"libcamerasrc + v4l2h264enc",
		"Camera H264 passthrough",
	}
	if raspi {
		options = []string{
			"Raspberry Pi v4l2h264enc",
			"VAAPI (vaapipostproc + vaapih264enc)",
			"libcamerasrc + v4l2h264enc",
			"Camera H264 passthrough",
		}
	}
	idx, err := promptChoice(reader, "Select Linux H264 mode", options)
	if err != nil {
		return LinuxH264VAAPI, err
	}
	if raspi {
		switch idx {
		case 0:
			return LinuxH264RaspiV4L2, nil
		case 1:
			return LinuxH264VAAPI, nil
		case 2:
			return LinuxH264Libcamera, nil
		default:
			return LinuxH264CameraH264, nil
		}
	}
	switch idx {
	case 1:
		return LinuxH264RaspiV4L2, nil
	case 2:
		return LinuxH264Libcamera, nil
	case 3:
		return LinuxH264CameraH264, nil
	default:
		return LinuxH264VAAPI, nil
	}
}

func promptAudioCodec(reader *bufio.Reader) (AudioCodec, error) {
	options := []string{
		"Opus (rtpopuspay)",
		"G.711 PCMU (rtppcmupay)",
	}
	idx, err := promptChoice(reader, "Select an audio codec", options)
	if err != nil {
		return AudioOpus, err
	}
	switch idx {
	case 1:
		return AudioPCMU, nil
	default:
		return AudioOpus, nil
	}
}

func promptFormat(reader *bufio.Reader) (string, error) {
	options := []string{
		"NV12",
		"I420",
		"Custom",
	}
	idx, err := promptChoice(reader, "Select a format", options)
	if err != nil {
		return "", err
	}
	switch idx {
	case 0:
		return "NV12", nil
	case 1:
		return "I420", nil
	default:
		return promptString(reader, "Format", "NV12")
	}
}

func addPipelineWatch(pipeline *gst.Pipeline, label string, mainLoop *glib.MainLoop, all []*gst.Pipeline) {
	pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		switch msg.Type() {
		case gst.MessageEOS: // When end-of-stream is received stop the main loop
			for _, p := range all {
				if p != nil {
					p.BlockSetState(gst.StateNull)
				}
			}
			mainLoop.Quit()
		case gst.MessageError: // Error messages are always fatal
			err := msg.ParseError()
			fmt.Printf("ERROR (%s): %s\n", label, err.Error())
			if debug := err.DebugString(); debug != "" {
				fmt.Println("DEBUG:", debug)
			}
			for _, p := range all {
				if p != nil {
					p.BlockSetState(gst.StateNull)
				}
			}
			mainLoop.Quit()
		default:
			// All messages implement a Stringer. However, this is
			// typically an expensive thing to do and should be avoided.
			fmt.Printf("[%s] %s\n", label, msg)
		}
		return true
	})
}

func devicePropPrefix(deviceProp string) string {
	if deviceProp == "" {
		return ""
	}
	return deviceProp + " "
}

func stringProp(values map[string]any, key string) string {
	if v, ok := values[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func intProp(values map[string]any, key string) int {
	if v, ok := values[key]; ok {
		if i, ok := toInt(v); ok {
			return i
		}
	}
	return -1
}

func detectPlatform() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "darwin", nil
	case "linux":
		return "linux", nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func detectLinuxVariant() LinuxVariant {
	data, err := os.ReadFile("/proc/device-tree/model")
	if err != nil {
		return LinuxGeneric
	}
	model := string(data)
	if strings.Contains(model, "Raspberry Pi") {
		return LinuxRaspi
	}
	if strings.Contains(model, "NVIDIA Jetson Nano") {
		return LinuxJetson
	}
	if strings.Contains(model, "Radxa ROCK 5") || strings.Contains(model, "Rock 5") || strings.Contains(model, "ROCK 5") {
		return LinuxRock5
	}
	return LinuxGeneric
}

func parseSelection(line string) (int, error) {
	fields := strings.FieldsFunc(line, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if len(fields) == 0 {
		return 0, errors.New("no selection")
	}
	return strconv.Atoi(fields[0])
}

func promptString(reader *bufio.Reader, prompt, def string) (string, error) {
	for {
		fmt.Printf("%s [%s]: ", prompt, def)
		line, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if line == "" {
			return def, nil
		}
		return line, nil
	}
}

func promptInt(reader *bufio.Reader, prompt string, def int) (int, error) {
	for {
		fmt.Printf("%s [%d]: ", prompt, def)
		line, err := readLine(reader)
		if err != nil {
			return 0, err
		}
		if line == "" {
			return def, nil
		}
		n, err := strconv.Atoi(line)
		if err != nil || n <= 0 {
			fmt.Println("Enter a positive number.")
			continue
		}
		return n, nil
	}
}

func promptPort(reader *bufio.Reader, prompt string, def int) (int, error) {
	for {
		port, err := promptInt(reader, prompt, def)
		if err != nil {
			return 0, err
		}
		if port < 1 || port > 65535 {
			fmt.Println("Enter a port between 1 and 65535.")
			continue
		}
		return port, nil
	}
}

func promptFraction(reader *bufio.Reader, prompt, def string) (string, error) {
	for {
		fmt.Printf("%s [%s]: ", prompt, def)
		line, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if line == "" {
			line = def
		}
		parsed, err := parseFraction(line)
		if err != nil {
			fmt.Println("Enter a fraction like 30/1.")
			continue
		}
		return parsed, nil
	}
}

func parseFraction(val string) (string, error) {
	parts := strings.Split(strings.TrimSpace(val), "/")
	if len(parts) != 2 {
		return "", errors.New("invalid fraction")
	}
	num, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", err
	}
	den, err := strconv.Atoi(parts[1])
	if err != nil || den == 0 {
		return "", errors.New("invalid fraction")
	}
	return fmt.Sprintf("%d/%d", num, den), nil
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		if len(line) == 0 {
			return "", err
		}
	}
	return strings.TrimSpace(line), nil
}

func toInt(val any) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	default:
		return 0, false
	}
}

func main() {
	examples.RunLoop(func(loop *glib.MainLoop) error {
		return runPipeline(loop)
	})
}
