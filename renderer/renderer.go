package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

type preparedLayer struct {
	layer     Layer
	localPath string
	width     int
	height    int
}

func generateVideo(tmpl Template, payload map[string]interface{}, outputPath, tmpDir string, log zerolog.Logger) error {
	expandedLayers, err := expandSemanticLayers(tmpl.Layers, payload, log)
	if err != nil {
		return fmt.Errorf("failed to expand layers: %w", err)
	}

	var ffmpegInputs []string
	var preparedLayers []preparedLayer

	for _, layer := range expandedLayers {
		if layer.Type == "text" && layer.TextContent == "" {
			return fmt.Errorf("renderer validation failed: layer '%s' has empty text_content", layer.Name)
		}
		pLayer := preparedLayer{layer: layer}
		if layer.Type == "image" {
			localPath, err := downloadFileFromURL(layer.File, tmpDir, layer.Name+"_")
			if err != nil {
				return err
			}

			if layer.Name == "background" && layer.Motion != nil {
				// 1. Generate the background motion
				bgVideo, err := renderBackgroundMotion(localPath, layer, tmpl, tmpDir, log)
				if err != nil {
					return fmt.Errorf("background motion render failed: %w", err)
				}

				// 2. Apply Post-Effect if requested (film, warm, etc.)
				if layer.PostEffect != "" {
					bgWithEffect, err := applyPostEffect(bgVideo, layer.PostEffect, tmpDir, log)
					if err != nil {
						return err
					}
					bgVideo = bgWithEffect
				}

				ffmpegInputs = append(ffmpegInputs, "-i", bgVideo)
			} else {
				ffmpegInputs = append(ffmpegInputs, "-loop", "1", "-framerate", fmt.Sprintf("%d", tmpl.FPS), "-t", fmt.Sprintf("%.3f", tmpl.DurationSec), "-i", localPath)
			}
			pLayer.localPath = localPath
		} else if layer.Type == "box" || layer.Type == "text" || strings.HasPrefix(layer.Type, "text_") || layer.Type == "icon" {
			outPath := filepath.Join(tmpDir, fmt.Sprintf("%s.png", layer.Name))
			if err := generateOverlayElement(&layer, tmpl.Canvas, outPath, log); err != nil {
				return err
			}
			ffmpegInputs = append(ffmpegInputs, "-loop", "1", "-framerate", fmt.Sprintf("%d", tmpl.FPS), "-t", fmt.Sprintf("%.3f", tmpl.DurationSec), "-i", outPath)
			pLayer.localPath = outPath
			pLayer.width, pLayer.height, _ = getPNGDimensions(outPath)
		}
		preparedLayers = append(preparedLayers, pLayer)
	}

	if len(preparedLayers) == 0 && len(tmpl.AudioTracks) == 0 {
		return fmt.Errorf("no layers or audio tracks to render")
	}

	var videoFilterComplex strings.Builder
	lastOutputStream := "[0:v]"
	if len(preparedLayers) > 0 {
		for i := 1; i < len(preparedLayers); i++ {
			inputStream := fmt.Sprintf("[%d:v]", i)
			overlayResult := buildOverlayFilter(lastOutputStream, inputStream, preparedLayers[i], i)
			if videoFilterComplex.Len() > 0 && overlayResult.Chain != "" {
				videoFilterComplex.WriteString(";")
			}
			videoFilterComplex.WriteString(overlayResult.Chain)
			lastOutputStream = overlayResult.OutputStream
		}
		videoFilterComplex.WriteString(fmt.Sprintf(";%sformat=yuv420p[v]", lastOutputStream))
	} else {
		ffmpegInputs = append(ffmpegInputs, "-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=%dx%d:d=%.3f:r=%d", tmpl.Canvas.Width, tmpl.Canvas.Height, tmpl.DurationSec, tmpl.FPS))
		videoFilterComplex.WriteString("[0:v]format=yuv420p[v]")
	}

	var audioFilterComplex string
	var audioMap string
	hasAudio := false
	if len(tmpl.AudioTracks) > 0 && tmpl.AudioTracks[0].File != "" {
		track := tmpl.AudioTracks[0]
		localAudioPath, err := downloadFileFromURL(track.File, tmpDir, "audio_")
		if err != nil {
			return fmt.Errorf("failed to download audio file: %w", err)
		}
		ffmpegInputs = append(ffmpegInputs, "-i", localAudioPath)
		audioInputIndex := len(preparedLayers)
		if len(preparedLayers) == 0 {
			audioInputIndex = 1
		}
		var filters []string
		audioInputStream := fmt.Sprintf("[%d:a]", audioInputIndex)
		if track.Loop {
			filters = append(filters, fmt.Sprintf("amovie=%s:loop=0,asetpts=N/SR/TB", localAudioPath))
			audioInputStream = ""
		}
		if track.StartAtSec > 0 {
			startMs := int(track.StartAtSec * 1000)
			filters = append(filters, fmt.Sprintf("adelay=%d|%d", startMs, startMs))
		}
		if track.Volume > 0 && track.Volume != 1.0 {
			filters = append(filters, fmt.Sprintf("volume=%.2f", track.Volume))
		}
		if len(filters) > 0 {
			audioFilterComplex = fmt.Sprintf(";%s%s[a]", audioInputStream, strings.Join(filters, ","))
		} else {
			audioFilterComplex = fmt.Sprintf(";%s[a]", audioInputStream)
		}
		audioMap = "-map"
		hasAudio = true
	}

	fullFilterComplex := videoFilterComplex.String() + audioFilterComplex
	ffmpegArgs := append(ffmpegInputs, "-filter_complex", fullFilterComplex)
	ffmpegArgs = append(ffmpegArgs, "-map", "[v]")
	if hasAudio {
		ffmpegArgs = append(ffmpegArgs, audioMap, "[a]")
	}
	ffmpegArgs = append(ffmpegArgs, "-t", fmt.Sprintf("%.3f", tmpl.DurationSec), "-c:v", "libx264", "-preset", "medium", "-profile:v", "high", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-crf", "22")
	if hasAudio {
		ffmpegArgs = append(ffmpegArgs, "-c:a", "aac", "-b:a", "128k", "-shortest")
	} else {
		ffmpegArgs = append(ffmpegArgs, "-an")
	}
	ffmpegArgs = append(ffmpegArgs, "-y", outputPath)

	log.Info().Str("cmd", "ffmpeg "+strings.Join(ffmpegArgs, " ")).Msg("Executing ffmpeg overlay pass")
	return runCommand("ffmpeg", ffmpegArgs...)
}

func expandSemanticLayers(layers []Layer, payload map[string]interface{}, log zerolog.Logger) ([]Layer, error) {
	var finalLayers []Layer
	textsPayload, _ := payload["texts"].([]interface{})

	for _, layer := range layers {
		switch layer.Type {
		case "topic", "title":
			newLayer := layer
			if layer.Type == "topic" {
				newLayer.Type = "text_topic"
				newLayer.TextContent, _ = payload["topic"].(string)
			} else {
				newLayer.Type = "text_title"
				newLayer.TextContent, _ = payload["title"].(string)
			}
			finalLayers = append(finalLayers, newLayer)

		case "text_carousel":
			if len(textsPayload) == 0 {
				continue
			}
			segmentDuration := (layer.EndSec - layer.StartSec) / float64(len(textsPayload))
			for i, text := range textsPayload {
				// Standard copy of the layer properties
				newLayer := Layer{
					Name:        fmt.Sprintf("%s_%d", layer.Name, i),
					Type:        "text_carousel_item",
					TextContent: text.(string),
					StartSec:    layer.StartSec + (float64(i) * segmentDuration),
					EndSec:      layer.StartSec + float64(i+1)*segmentDuration,

					// Pass through all styling properties
					Pos:         layer.Pos,
					Gravity:     layer.Gravity,
					Font:        layer.Font,
					BoundingBox: layer.BoundingBox,
					Shadow:      layer.Shadow,
					Box:         layer.Box,

					Animations: make([]Animation, len(layer.Animations)),
				}

				// Adjust animation start times relative to the new layer's start
				for j, anim := range layer.Animations {
					newAnim := anim
					if newAnim.StartSec == 0.0 && newAnim.Type != "fade_out" {
						newAnim.StartSec = newLayer.StartSec
					}
					newLayer.Animations[j] = newAnim
				}
				finalLayers = append(finalLayers, newLayer)
			}

		case "pager":
			if len(textsPayload) < 2 {
				continue
			}
			segmentDuration := (layer.EndSec - layer.StartSec) / float64(len(textsPayload))
			for i := 0; i < len(textsPayload); i++ {
				segLayer := layer
				segLayer.Name = fmt.Sprintf("%s_%d", layer.Name, i)
				segLayer.Type = "text_pager_item"
				segLayer.TextContent = fmt.Sprintf("%d/%d", i+1, len(textsPayload))
				segLayer.StartSec = layer.StartSec + (float64(i) * segmentDuration)
				segLayer.EndSec = segLayer.StartSec + segmentDuration
				finalLayers = append(finalLayers, segLayer)
			}

		default:
			finalLayers = append(finalLayers, layer)
		}
	}
	return finalLayers, nil
}

func buildOverlayFilter(baseStream, overlayStream string, pLayer preparedLayer, index int) overlayResult {
	layer := pLayer.layer

	xDefault, yDefault := calculateOverlayPosition(pLayer)

	// We track the "Current Expression" and the "Last Known Value" (as a string for FFmpeg math)
	// to allow chaining multiple moves.
	xExpr := xDefault
	yExpr := yDefault

	lastXVal := xDefault
	lastYVal := yDefault

	var animationFilters []string

	// Image Sizing & Fitting Logic
	if layer.Width > 0 && layer.Height > 0 {
		mode := "stretch"
		if layer.Sizing != nil && layer.Sizing.Mode != "" {
			mode = layer.Sizing.Mode
		}

		if mode == "cover" {
			animationFilters = append(animationFilters,
				fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase", layer.Width, layer.Height),
				fmt.Sprintf("crop=%d:%d", layer.Width, layer.Height),
			)
		} else if mode == "fit" {
			animationFilters = append(animationFilters,
				fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", layer.Width, layer.Height),
			)
		} else {
			animationFilters = append(animationFilters, fmt.Sprintf("scale=%d:%d", layer.Width, layer.Height))
		}
	} else if layer.Width > 0 {
		animationFilters = append(animationFilters, fmt.Sprintf("scale=%d:-1", layer.Width))
	} else if layer.Height > 0 {
		animationFilters = append(animationFilters, fmt.Sprintf("scale=-1:%d", layer.Height))
	}

	for _, anim := range layer.Animations {
		animationStartTime := anim.StartSec
		if anim.StartSec == 0.0 && anim.Type != "fade_out" {
			animationStartTime = layer.StartSec
		}
		var filter string
		switch anim.Type {
		case "pop_in":
			p := fmt.Sprintf("clip((t-%.3f)/%.3f,0,1)", animationStartTime, anim.DurationSec)
			easeOutBack := fmt.Sprintf("(1+2.70158*pow(%s-1,3)+1.70158*pow(%s-1,2))", p, p)
			scaleExpr := fmt.Sprintf("0.8 + 0.2 * %s", easeOutBack)
			filter = fmt.Sprintf("scale=w='iw*(%s)':h='ih*(%s)':eval=frame", scaleExpr, scaleExpr)

		case "fade_in":
			filter = fmt.Sprintf("fade=type=in:start_time=%.3f:duration=%.3f:alpha=1", animationStartTime, anim.DurationSec)

		case "fade_out":
			fadeOutStart := layer.EndSec - anim.DurationSec
			filter = fmt.Sprintf("fade=type=out:start_time=%.3f:duration=%.3f:alpha=1", fadeOutStart, anim.DurationSec)

		case "slide_in":
			// SLIDE IN (simplified for single-use entry)
			linearP := fmt.Sprintf("clip((t-%.3f)/%.3f,0,1)", animationStartTime, anim.DurationSec)
			progress := fmt.Sprintf("(1-pow(1-%s,4))", linearP)

			h := pLayer.height
			w := pLayer.width
			if layer.Height > 0 {
				h = layer.Height
			}
			if layer.Width > 0 {
				w = layer.Width
			}

			startY := fmt.Sprintf("(-%d)", h)
			startX := fmt.Sprintf("(-%d)", w)

			if anim.Direction == "bottom" {
				startY = "H"
			}
			if anim.Direction == "right" {
				startX = "W"
			}

			if anim.Distance != nil {
				dist := *anim.Distance
				switch anim.Direction {
				case "top":
					startY = fmt.Sprintf("((%s) - %d)", yDefault, dist)
				case "bottom":
					startY = fmt.Sprintf("((%s) + %d)", yDefault, dist)
				case "left":
					startX = fmt.Sprintf("((%s) - %d)", xDefault, dist)
				case "right":
					startX = fmt.Sprintf("((%s) + %d)", xDefault, dist)
				}
			}

			// Note: slide_in overwrites previous position logic, best used as the first animation
			if anim.Direction == "top" || anim.Direction == "bottom" {
				yExpr = fmt.Sprintf("((%s) + ((%s) - (%s)) * %s)", startY, yDefault, startY, progress)
				lastYVal = yDefault // Update state assuming we end at default
			} else {
				xExpr = fmt.Sprintf("((%s) + ((%s) - (%s)) * %s)", startX, xDefault, startX, progress)
				lastXVal = xDefault
			}
			continue

		case "move":
			// MOVE (Chained Logic)
			if anim.TargetX != nil || anim.TargetY != nil {
				t := fmt.Sprintf("clip((t-%.3f)/%.3f,0,1)", animationStartTime, anim.DurationSec)
				progress := fmt.Sprintf("(1-pow(1-%s,4))", t) // Default Ease Out

				if anim.Easing == "linear" {
					progress = t
				} else if anim.Easing == "ease_in" {
					progress = fmt.Sprintf("pow(%s,4)", t)
				}

				endTime := animationStartTime + anim.DurationSec

				// CHAINING LOGIC:
				// We wrap the *previous* expression (xExpr) in an IF statement.
				// If time < Start: Use Previous Expression (preserves history).
				// If time > End: Snap to Target (sets up for next move).
				// Else: Interpolate from LastVal to Target.

				if anim.TargetX != nil {
					target := *anim.TargetX
					targetStr := fmt.Sprintf("%d", target)

					// Interpolate from lastXVal -> target
					lerp := fmt.Sprintf("((%s) + (%d - (%s)) * %s)", lastXVal, target, lastXVal, progress)

					xExpr = fmt.Sprintf("if(lt(t,%.3f),(%s),if(gt(t,%.3f),%d,(%s)))",
						animationStartTime, xExpr, endTime, target, lerp)

					lastXVal = targetStr // Update state for next chain
				}

				if anim.TargetY != nil {
					target := *anim.TargetY
					targetStr := fmt.Sprintf("%d", target)

					lerp := fmt.Sprintf("((%s) + (%d - (%s)) * %s)", lastYVal, target, lastYVal, progress)

					yExpr = fmt.Sprintf("if(lt(t,%.3f),(%s),if(gt(t,%.3f),%d,(%s)))",
						animationStartTime, yExpr, endTime, target, lerp)

					lastYVal = targetStr
				}
			}
			continue

		default:
			continue
		}
		animationFilters = append(animationFilters, filter)
	}

	animatedStream := overlayStream
	var fullFilterChain string
	if len(animationFilters) > 0 {
		fullFilterChain = fmt.Sprintf("%s%s[anim%d];", overlayStream, strings.Join(animationFilters, ","), index)
		animatedStream = fmt.Sprintf("[anim%d]", index)
	}

	finalOutputStream := fmt.Sprintf("[s%d_out]", index)

	overlayFilter := fmt.Sprintf("%s%soverlay=x='%s':y='%s':format=auto:enable='between(t,%.3f,%.3f)'%s",
		baseStream, animatedStream, xExpr, yExpr, layer.StartSec, layer.EndSec, finalOutputStream)

	fullChain := fullFilterChain + overlayFilter
	return overlayResult{Chain: fullChain, OutputStream: finalOutputStream}
}

func renderBackgroundMotion(inputPath string, layer Layer, tmpl Template, tmpDir string, log zerolog.Logger) (string, error) {
	outputPath := filepath.Join(tmpDir, "bg_motion.mp4")
	preprocessedPath := filepath.Join(tmpDir, "bg_preprocessed.png")

	canvasW, canvasH := tmpl.Canvas.Width, tmpl.Canvas.Height
	dur := tmpl.DurationSec
	fps := tmpl.FPS
	totalFrames := int(math.Round(dur * float64(fps)))

	targetW := canvasW * 2
	targetH := canvasH * 2
	cmd := exec.Command("magick", "convert", inputPath,
		"-resize", fmt.Sprintf("%dx%d^", targetW, targetH),
		"-gravity", "center",
		"-extent", fmt.Sprintf("%dx%d", targetW, targetH),
		preprocessedPath,
	)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("imagemagick aspect-ratio preprocessing failed: %w", err)
	}

	var zoomExpr, xExpr, yExpr string
	pi := "3.1415926535"

	motionType := "linear"
	if layer.Motion != nil && layer.Motion.Type != "" {
		motionType = layer.Motion.Type
	}

	if motionType == "drift" && layer.Motion != nil {
		log.Info().Msg("Using 'drift' background motion (Hypnotic Spiral)")

		// ALGORITHM: THE HYPNOTIC SPIRAL
		// 1. Zoom: Always moves in ONE direction (In or Out). No yo-yo.
		//    We use a sine wave with a massive period (4x duration) so we only traverse the
		//    first 1/4 of the curve (the almost-linear acceleration phase).
		// 2. Pan: Moves in a wide Ellipse/Circle.
		//    X uses Sine, Y uses Cosine. This circular motion combined with Zoom
		//    creates a "Vortex" feel that implies 3D depth.

		var zoomPeriodMult float64
		var panPeriodMult float64

		switch layer.Motion.Speed {
		case "fast":
			zoomPeriodMult = 2.0 // Faster push
			panPeriodMult = 1.0  // 1 full circle per duration
		case "slow":
			zoomPeriodMult = 6.0 // Extremely slow creep
			panPeriodMult = 3.0  // 1/3rd of a circle
		default: // medium
			zoomPeriodMult = 4.0 // Standard documentary push
			panPeriodMult = 2.0  // Half circle
		}

		// Calculate Periods based on video duration
		zoomPeriod := dur * zoomPeriodMult
		panPeriod := dur * panPeriodMult

		// ZOOM OSCILLATOR (Unidirectional Push)
		// By default, we start at 0 and move towards 1. (Zoom In)
		p_zoom := fmt.Sprintf("(sin(2*%s*on/(%f*%d)))", pi, zoomPeriod, fps)

		// PAN OSCILLATORS (Circular Orbit)
		// X uses Sine, Y uses Cosine. This phase difference creates a circle.
		p_x := fmt.Sprintf("0.5 + 0.5*sin(2*%s*on/(%f*%d))", pi, panPeriod, fps)
		p_y := fmt.Sprintf("0.5 + 0.5*cos(2*%s*on/(%f*%d))", pi, panPeriod, fps)

		// Apply Ranges
		// Tight Zoom range prevents "dizzying" speed.
		zoomMin, zoomMax := 1.0, 1.25
		if len(layer.Motion.ZoomRange) == 2 {
			zoomMin, zoomMax = layer.Motion.ZoomRange[0], layer.Motion.ZoomRange[1]
		}

		// Standard formula: Min + (Max-Min) * Progress
		zoomExpr = fmt.Sprintf("%f+(%f-%f)*%s", zoomMin, zoomMax, zoomMin, p_zoom)

		// Pan ranges define the "box" the camera circles within.
		xMin, xMax := 0.40, 0.60
		if len(layer.Motion.PanXRange) == 2 {
			xMin, xMax = layer.Motion.PanXRange[0], layer.Motion.PanXRange[1]
		}
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(%s))", canvasW, xMin, xMax, xMin, p_x)

		yMin, yMax := 0.40, 0.60
		if len(layer.Motion.PanYRange) == 2 {
			yMin, yMax = layer.Motion.PanYRange[0], layer.Motion.PanYRange[1]
		}
		yExpr = fmt.Sprintf("(ih-%d/zoom)*(%f+(%f-%f)*(%s))", canvasH, yMin, yMax, yMin, p_y)

	} else if motionType == "pulse" && layer.Motion != nil {
		log.Info().Msg("Using 'pulse' background motion")
		period := 5.0
		if layer.Motion.Speed == "slow" {
			period = 8.0
		}
		if layer.Motion.Speed == "fast" {
			period = 3.0
		}
		osc := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, period, fps)
		zoomMin, zoomMax := 1.0, 1.15
		if len(layer.Motion.ZoomRange) == 2 {
			zoomMin, zoomMax = layer.Motion.ZoomRange[0], layer.Motion.ZoomRange[1]
		}
		zoomExpr = fmt.Sprintf("%f+(%f-%f)*(%s)", zoomMin, zoomMax, zoomMin, osc)
		xExpr = fmt.Sprintf("(iw-%d/zoom)/2", canvasW)
		yExpr = fmt.Sprintf("(ih-%d/zoom)/2", canvasH)

	} else if motionType == "glance" && layer.Motion != nil {
		log.Info().Msg("Using 'glance' background motion")
		period := 10.0
		if layer.Motion.Speed == "slow" {
			period = 15.0
		}
		if layer.Motion.Speed == "fast" {
			period = 7.0
		}
		osc := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, period, fps)
		zoomVal := 1.15
		if len(layer.Motion.ZoomRange) > 0 {
			zoomVal = layer.Motion.ZoomRange[0]
		}
		zoomExpr = fmt.Sprintf("%f", zoomVal)
		xMin, xMax := 0.40, 0.60
		if len(layer.Motion.PanXRange) == 2 {
			xMin, xMax = layer.Motion.PanXRange[0], layer.Motion.PanXRange[1]
		}
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(%s))", canvasW, xMin, xMax, xMin, osc)
		yExpr = fmt.Sprintf("(ih-%d/zoom)/2", canvasH)

	} else {
		log.Info().Msg("Using 'linear' background motion")
		zoomStart, zoomEnd := 1.0, 1.05
		if layer.Motion != nil && layer.Motion.ZoomStart != 0 {
			zoomStart, zoomEnd = layer.Motion.ZoomStart, layer.Motion.ZoomEnd
		}
		panXStart, panXEnd := 0.48, 0.52
		if layer.Motion != nil && layer.Motion.PanStart.X != 0 {
			panXStart, panXEnd = layer.Motion.PanStart.X, layer.Motion.PanEnd.X
		}
		panYStart, panYEnd := 0.48, 0.52
		if layer.Motion != nil && layer.Motion.PanStart.Y != 0 {
			panYStart, panYEnd = layer.Motion.PanStart.Y, layer.Motion.PanEnd.Y
		}
		zoomExpr = fmt.Sprintf("%f+%f*(on/%d)", zoomStart, (zoomEnd - zoomStart), totalFrames-1)
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(on/%d))", canvasW, panXStart, panXEnd, panXStart, totalFrames-1)
		yExpr = fmt.Sprintf("(ih-%d/zoom)*(%f+(%f-%f)*(on/%d))", canvasH, panYStart, panYEnd, panYStart, totalFrames-1)
	}

	filter := fmt.Sprintf("zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d,scale=%dx%d:flags=bicubic,eq=brightness=%0.2f[v]",
		zoomExpr, xExpr, yExpr, totalFrames,
		int(float64(canvasW)*1.25), int(float64(canvasH)*1.25),
		fps, canvasW, canvasH, getBrightness(layer))

	args := []string{
		"-loop", "1", "-i", preprocessedPath,
		"-filter_complex", filter, "-map", "[v]", "-t", fmt.Sprintf("%.3f", dur),
		"-c:v", "libx264", "-preset", "medium", "-profile:v", "high", "-level:v", "4.0", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-crf", "22",
		"-y", outputPath,
	}

	log.Info().Str("cmd", "ffmpeg "+strings.Join(args, " ")).Msg("Executing background render")
	if err := runCommand("ffmpeg", args...); err != nil {
		return "", fmt.Errorf("background motion render failed: %w", err)
	}
	return outputPath, nil
}

/*
func renderBackgroundMotion(inputPath string, layer Layer, tmpl Template, tmpDir string, log zerolog.Logger) (string, error) {
	outputPath := filepath.Join(tmpDir, "bg_motion.mp4")
	preprocessedPath := filepath.Join(tmpDir, "bg_preprocessed.png")

	canvasW, canvasH := tmpl.Canvas.Width, tmpl.Canvas.Height
	dur := tmpl.DurationSec
	fps := tmpl.FPS
	totalFrames := int(math.Round(dur * float64(fps)))

	targetW := canvasW * 2
	targetH := canvasH * 2
	cmd := exec.Command("magick", "convert", inputPath,
		"-resize", fmt.Sprintf("%dx%d^", targetW, targetH),
		"-gravity", "center",
		"-extent", fmt.Sprintf("%dx%d", targetW, targetH),
		preprocessedPath,
	)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("imagemagick aspect-ratio preprocessing failed: %w", err)
	}

	var zoomExpr, xExpr, yExpr string
	pi := "3.1415926535"

	motionType := "linear"
	if layer.Motion != nil && layer.Motion.Type != "" {
		motionType = layer.Motion.Type
	}

	if motionType == "drift" && layer.Motion != nil {
		log.Info().Msg("Using 'drift' background motion (Lissajous Path)")

		// LISSAJOUS STRATEGY:
		// We use different periods for X, Y, and Z to create a non-repeating, smooth path.
		// Crucially, the Zoom period is made very long to minimize the "Zoom Out" feeling.

		var pZ, pX, pY float64
		switch layer.Motion.Speed {
		case "fast":
			pZ, pX, pY = 30.0, 15.0, 10.0
		case "slow":
			pZ, pX, pY = 90.0, 45.0, 35.0
		default: // medium
			// Zoom cycle is 60s (very slow breathing)
			// Pan X cycle is 23s
			// Pan Y cycle is 19s
			// These primes ensure the path doesn't repeat or loop obviously.
			pZ, pX, pY = 60.0, 23.0, 19.0
		}

		// OSCILLATORS
		// Using 'on' (frame count) for stability.
		// Zoom: Standard Sine wave, but very slow.
		oscZ := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, pZ, fps)

		// Pan X: Sine wave
		oscX := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, pX, fps)

		// Pan Y: Cosine wave (starts at 1.0 instead of 0.0).
		// This 90-degree phase shift is what turns a diagonal line into a circular/oval motion.
		oscY := fmt.Sprintf("(cos(2*%s*on/(%f*%d))+1)/2", pi, pY, fps)

		// Apply Ranges
		zoomMin, zoomMax := 1.05, 1.15
		if len(layer.Motion.ZoomRange) == 2 {
			zoomMin, zoomMax = layer.Motion.ZoomRange[0], layer.Motion.ZoomRange[1]
		}
		zoomExpr = fmt.Sprintf("%f+(%f-%f)*(%s)", zoomMin, zoomMax, zoomMin, oscZ)

		xMin, xMax := 0.45, 0.55
		if len(layer.Motion.PanXRange) == 2 {
			xMin, xMax = layer.Motion.PanXRange[0], layer.Motion.PanXRange[1]
		}
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(%s))", canvasW, xMin, xMax, xMin, oscX)

		yMin, yMax := 0.48, 0.52
		if len(layer.Motion.PanYRange) == 2 {
			yMin, yMax = layer.Motion.PanYRange[0], layer.Motion.PanYRange[1]
		}
		yExpr = fmt.Sprintf("(ih-%d/zoom)*(%f+(%f-%f)*(%s))", canvasH, yMin, yMax, yMin, oscY)

	} else if motionType == "pulse" && layer.Motion != nil {
		log.Info().Msg("Using 'pulse' background motion")
		period := 5.0
		if layer.Motion.Speed == "slow" {
			period = 8.0
		}
		if layer.Motion.Speed == "fast" {
			period = 3.0
		}
		osc := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, period, fps)
		zoomMin, zoomMax := 1.0, 1.15
		if len(layer.Motion.ZoomRange) == 2 {
			zoomMin, zoomMax = layer.Motion.ZoomRange[0], layer.Motion.ZoomRange[1]
		}
		zoomExpr = fmt.Sprintf("%f+(%f-%f)*(%s)", zoomMin, zoomMax, zoomMin, osc)
		xExpr = fmt.Sprintf("(iw-%d/zoom)/2", canvasW)
		yExpr = fmt.Sprintf("(ih-%d/zoom)/2", canvasH)

	} else if motionType == "glance" && layer.Motion != nil {
		log.Info().Msg("Using 'glance' background motion")
		period := 10.0
		if layer.Motion.Speed == "slow" {
			period = 15.0
		}
		if layer.Motion.Speed == "fast" {
			period = 7.0
		}
		osc := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, period, fps)
		zoomVal := 1.15
		if len(layer.Motion.ZoomRange) > 0 {
			zoomVal = layer.Motion.ZoomRange[0]
		}
		zoomExpr = fmt.Sprintf("%f", zoomVal)
		xMin, xMax := 0.40, 0.60
		if len(layer.Motion.PanXRange) == 2 {
			xMin, xMax = layer.Motion.PanXRange[0], layer.Motion.PanXRange[1]
		}
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(%s))", canvasW, xMin, xMax, xMin, osc)
		yExpr = fmt.Sprintf("(ih-%d/zoom)/2", canvasH)

	} else {
		log.Info().Msg("Using 'linear' background motion")
		zoomStart, zoomEnd := 1.0, 1.05
		if layer.Motion != nil && layer.Motion.ZoomStart != 0 {
			zoomStart, zoomEnd = layer.Motion.ZoomStart, layer.Motion.ZoomEnd
		}
		panXStart, panXEnd := 0.48, 0.52
		if layer.Motion != nil && layer.Motion.PanStart.X != 0 {
			panXStart, panXEnd = layer.Motion.PanStart.X, layer.Motion.PanEnd.X
		}
		panYStart, panYEnd := 0.48, 0.52
		if layer.Motion != nil && layer.Motion.PanStart.Y != 0 {
			panYStart, panYEnd = layer.Motion.PanStart.Y, layer.Motion.PanEnd.Y
		}
		zoomExpr = fmt.Sprintf("%f+%f*(on/%d)", zoomStart, (zoomEnd - zoomStart), totalFrames-1)
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(on/%d))", canvasW, panXStart, panXEnd, panXStart, totalFrames-1)
		yExpr = fmt.Sprintf("(ih-%d/zoom)*(%f+(%f-%f)*(on/%d))", canvasH, panYStart, panYEnd, panYStart, totalFrames-1)
	}

	filter := fmt.Sprintf("zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d,scale=%dx%d:flags=bicubic,eq=brightness=%0.2f[v]",
		zoomExpr, xExpr, yExpr, totalFrames,
		int(float64(canvasW)*1.25), int(float64(canvasH)*1.25),
		fps, canvasW, canvasH, getBrightness(layer))

	args := []string{
		"-loop", "1", "-i", preprocessedPath,
		"-filter_complex", filter, "-map", "[v]", "-t", fmt.Sprintf("%.3f", dur),
		"-c:v", "libx264", "-preset", "medium", "-profile:v", "high", "-level:v", "4.0", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-crf", "22",
		"-y", outputPath,
	}

	log.Info().Str("cmd", "ffmpeg "+strings.Join(args, " ")).Msg("Executing background render")
	if err := runCommand("ffmpeg", args...); err != nil {
		return "", fmt.Errorf("background motion render failed: %w", err)
	}
	return outputPath, nil
} */

/* func renderBackgroundMotion(inputPath string, layer Layer, tmpl Template, tmpDir string, log zerolog.Logger) (string, error) {
	outputPath := filepath.Join(tmpDir, "bg_motion.mp4")
	preprocessedPath := filepath.Join(tmpDir, "bg_preprocessed.png")

	canvasW, canvasH := tmpl.Canvas.Width, tmpl.Canvas.Height
	dur := tmpl.DurationSec
	fps := tmpl.FPS
	totalFrames := int(math.Round(dur * float64(fps)))

	targetW := canvasW * 2
	targetH := canvasH * 2
	cmd := exec.Command("magick", "convert", inputPath,
		"-resize", fmt.Sprintf("%dx%d^", targetW, targetH),
		"-gravity", "center",
		"-extent", fmt.Sprintf("%dx%d", targetW, targetH),
		preprocessedPath,
	)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("imagemagick aspect-ratio preprocessing failed: %w", err)
	}

	var zoomExpr, xExpr, yExpr string
	pi := "3.1415926535"

	motionType := "linear"
	if layer.Motion != nil && layer.Motion.Type != "" {
		motionType = layer.Motion.Type
	}

	if motionType == "drift" && layer.Motion != nil {
		log.Info().Msg("Using 'drift' background motion")
		var pZ1, pX1, pY1, pZ2, pX2, pY2 float64
		switch layer.Motion.Speed {
		case "fast":
			pZ1, pX1, pY1, pZ2, pX2, pY2 = 7, 11, 13, 3, 5, 7
		case "slow":
			pZ1, pX1, pY1, pZ2, pX2, pY2 = 23, 29, 31, 11, 13, 17
		default:
			pZ1, pX1, pY1, pZ2, pX2, pY2 = 13, 17, 19, 5, 7, 11
		}

		oscZ1 := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, pZ1, fps)
		oscX1 := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, pX1, fps)
		oscY1 := fmt.Sprintf("(cos(2*%s*on/(%f*%d))+1)/2", pi, pY1, fps)
		oscZ2 := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, pZ2, fps)
		oscX2 := fmt.Sprintf("(cos(2*%s*on/(%f*%d))+1)/2", pi, pX2, fps)
		oscY2 := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, pY2, fps)

		p_z := fmt.Sprintf("0.8*%s + 0.2*%s", oscZ1, oscZ2)
		p_x := fmt.Sprintf("0.8*%s + 0.2*%s", oscX1, oscX2)
		p_y := fmt.Sprintf("0.8*%s + 0.2*%s", oscY1, oscY2)

		zoomMin, zoomMax := 1.05, 1.20
		if len(layer.Motion.ZoomRange) == 2 {
			zoomMin, zoomMax = layer.Motion.ZoomRange[0], layer.Motion.ZoomRange[1]
		}
		zoomExpr = fmt.Sprintf("%f+(%f-%f)*(%s)", zoomMin, zoomMax, zoomMin, p_z)

		xMin, xMax := 0.45, 0.55
		if len(layer.Motion.PanXRange) == 2 {
			xMin, xMax = layer.Motion.PanXRange[0], layer.Motion.PanXRange[1]
		}
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(%s))", canvasW, xMin, xMax, xMin, p_x)

		yMin, yMax := 0.48, 0.52
		if len(layer.Motion.PanYRange) == 2 {
			yMin, yMax = layer.Motion.PanYRange[0], layer.Motion.PanYRange[1]
		}
		yExpr = fmt.Sprintf("(ih-%d/zoom)*(%f+(%f-%f)*(%s))", canvasH, yMin, yMax, yMin, p_y)

	} else if motionType == "pulse" && layer.Motion != nil {
		log.Info().Msg("Using 'pulse' background motion")
		period := 5.0
		if layer.Motion.Speed == "slow" {
			period = 8.0
		}
		if layer.Motion.Speed == "fast" {
			period = 3.0
		}
		osc := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, period, fps)
		zoomMin, zoomMax := 1.0, 1.15
		if len(layer.Motion.ZoomRange) == 2 {
			zoomMin, zoomMax = layer.Motion.ZoomRange[0], layer.Motion.ZoomRange[1]
		}
		zoomExpr = fmt.Sprintf("%f+(%f-%f)*(%s)", zoomMin, zoomMax, zoomMin, osc)
		xExpr = fmt.Sprintf("(iw-%d/zoom)/2", canvasW)
		yExpr = fmt.Sprintf("(ih-%d/zoom)/2", canvasH)
	} else if motionType == "glance" && layer.Motion != nil {
		log.Info().Msg("Using 'glance' background motion")
		period := 10.0
		if layer.Motion.Speed == "slow" {
			period = 15.0
		}
		if layer.Motion.Speed == "fast" {
			period = 7.0
		}
		osc := fmt.Sprintf("(sin(2*%s*on/(%f*%d))+1)/2", pi, period, fps)
		zoomVal := 1.15
		if len(layer.Motion.ZoomRange) > 0 {
			zoomVal = layer.Motion.ZoomRange[0]
		}
		zoomExpr = fmt.Sprintf("%f", zoomVal)
		xMin, xMax := 0.40, 0.60
		if len(layer.Motion.PanXRange) == 2 {
			xMin, xMax = layer.Motion.PanXRange[0], layer.Motion.PanXRange[1]
		}
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(%s))", canvasW, xMin, xMax, xMin, osc)
		yExpr = fmt.Sprintf("(ih-%d/zoom)/2", canvasH)
	} else {
		log.Info().Msg("Using 'linear' background motion")
		zoomStart, zoomEnd := 1.0, 1.05
		if layer.Motion != nil && layer.Motion.ZoomStart != 0 {
			zoomStart, zoomEnd = layer.Motion.ZoomStart, layer.Motion.ZoomEnd
		}
		panXStart, panXEnd := 0.48, 0.52
		if layer.Motion != nil && layer.Motion.PanStart.X != 0 {
			panXStart, panXEnd = layer.Motion.PanStart.X, layer.Motion.PanEnd.X
		}
		panYStart, panYEnd := 0.48, 0.52
		if layer.Motion != nil && layer.Motion.PanStart.Y != 0 {
			panYStart, panYEnd = layer.Motion.PanStart.Y, layer.Motion.PanEnd.Y
		}
		zoomExpr = fmt.Sprintf("%f+%f*(on/%d)", zoomStart, (zoomEnd - zoomStart), totalFrames-1)
		xExpr = fmt.Sprintf("(iw-%d/zoom)*(%f+(%f-%f)*(on/%d))", canvasW, panXStart, panXEnd, panXStart, totalFrames-1)
		yExpr = fmt.Sprintf("(ih-%d/zoom)*(%f+(%f-%f)*(on/%d))", canvasH, panYStart, panYEnd, panYStart, totalFrames-1)
	}

	filter := fmt.Sprintf("zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d,scale=%dx%d:flags=bicubic,eq=brightness=%0.2f[v]",
		zoomExpr, xExpr, yExpr, totalFrames,
		int(float64(canvasW)*1.25), int(float64(canvasH)*1.25),
		fps, canvasW, canvasH, getBrightness(layer))

	args := []string{
		"-loop", "1", "-i", preprocessedPath,
		"-filter_complex", filter, "-map", "[v]", "-t", fmt.Sprintf("%.3f", dur),
		"-c:v", "libx264", "-preset", "medium", "-profile:v", "high", "-level:v", "4.0", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-crf", "22",
		"-y", outputPath,
	}

	log.Info().Str("cmd", "ffmpeg "+strings.Join(args, " ")).Msg("Executing background render")
	if err := runCommand("ffmpeg", args...); err != nil {
		return "", fmt.Errorf("background motion render failed: %w", err)
	}
	return outputPath, nil
}  */

func getBrightness(layer Layer) float64 {
	if layer.Filters != nil {
		if b, ok := (*layer.Filters)["brightness"].(float64); ok {
			return b
		}
	}
	return 0
}

type overlayResult struct{ Chain, OutputStream string }

func calculateOverlayPosition(pLayer preparedLayer) (string, string) {
	layer := pLayer.layer
	switch layer.Gravity {
	case "North":
		return fmt.Sprintf("%d-(w/2)", layer.Pos.X), fmt.Sprintf("%d", layer.Pos.Y)
	case "South":
		return fmt.Sprintf("%d-(w/2)", layer.Pos.X), fmt.Sprintf("%d-h", layer.Pos.Y)
	case "West":
		return fmt.Sprintf("%d", layer.Pos.X), fmt.Sprintf("%d-(h/2)", layer.Pos.Y)
	case "East":
		return fmt.Sprintf("%d-w", layer.Pos.X), fmt.Sprintf("%d-(h/2)", layer.Pos.Y)

	case "NorthWest":
		return fmt.Sprintf("%d", layer.Pos.X), fmt.Sprintf("%d", layer.Pos.Y)
	case "NorthEast":
		return fmt.Sprintf("W-%d-w", layer.Pos.X), fmt.Sprintf("%d", layer.Pos.Y)
	case "SouthWest":
		return fmt.Sprintf("%d", layer.Pos.X), fmt.Sprintf("H-%d-h", layer.Pos.Y)
	case "SouthEast":
		return fmt.Sprintf("W-%d-w", layer.Pos.X), fmt.Sprintf("H-%d-h", layer.Pos.Y)

	case "center":
		return fmt.Sprintf("%d-(w/2)", layer.Pos.X), fmt.Sprintf("%d-(h/2)", layer.Pos.Y)

	default:
		return fmt.Sprintf("%d", layer.Pos.X), fmt.Sprintf("%d", layer.Pos.Y)
	}
}

func generateOverlayElement(layer *Layer, canvas Canvas, outputPath string, log zerolog.Logger) error {
	var mainElementPath string
	tmpDir := filepath.Dir(outputPath)
	tmpPath := filepath.Join(tmpDir, "raw_"+layer.Name+".png")

	if layer.Type == "text" || strings.HasPrefix(layer.Type, "text_") {
		if layer.TextContent == "" {
			return generateEmptyPNG(outputPath)
		}
		if layer.Font == nil {
			return fmt.Errorf("layer '%s' is type text but has no font object", layer.Name)
		}

		fontPath, err := downloadAsset(layer.Font.File, tmpDir)
		if err != nil {
			return err
		}

		args := []string{"-background", "none", "-fill", layer.Font.Color, "-font", fontPath}

		if layer.BoundingBox != nil {
			if layer.BoundingBox.Height > 0 {
				// CONSTRAIN MODE: Omit -pointsize
				sizeArg := fmt.Sprintf("%dx%d", layer.BoundingBox.Width, layer.BoundingBox.Height)
				args = append(args, "-size", sizeArg, "-gravity", "center")
				if layer.Font.Align != "" {
					args = append(args, "-define", "caption:gravity="+layer.Font.Align)
				}
				args = append(args, "caption:"+layer.TextContent, tmpPath)
			} else {
				// WRAP MODE: Include -pointsize
				args = append(args, "-pointsize", strconv.Itoa(layer.Font.Size))
				sizeArg := fmt.Sprintf("%dx", layer.BoundingBox.Width)
				args = append(args, "-size", sizeArg)
				align := "North"
				if layer.Font.Align != "" {
					switch layer.Font.Align {
					case "left":
						align = "NorthWest"
					case "center":
						align = "North"
					case "right":
						align = "NorthEast"
					}
				}
				args = append(args, "-gravity", align)
				args = append(args, "caption:"+layer.TextContent, tmpPath)
			}
		} else {
			// LABEL MODE: Include -pointsize
			args = append(args, "-pointsize", strconv.Itoa(layer.Font.Size))
			args = append(args, "label:"+layer.TextContent, tmpPath)
		}

		if err := runCommand("magick", args...); err != nil {
			return err
		}
		trimArgs := []string{tmpPath, "-trim", "+repage", tmpPath}
		if err := runCommand("magick", trimArgs...); err != nil {
			return err
		}
		mainElementPath = tmpPath

	} else if layer.Type == "icon" {
		iconPath, err := downloadAsset(layer.File, tmpDir)
		if err != nil {
			return err
		}
		size := ""
		if layer.Sizing != nil {
			if layer.Sizing.Width > 0 && layer.Sizing.Height > 0 {
				size = fmt.Sprintf("%dx%d", layer.Sizing.Width, layer.Sizing.Height)
			} else if layer.Sizing.Width > 0 {
				size = fmt.Sprintf("%dx", layer.Sizing.Width)
			} else if layer.Sizing.Height > 0 {
				size = fmt.Sprintf("x%d", layer.Sizing.Height)
			}
		}
		args := []string{"-background", "none", iconPath}
		if layer.Font != nil && layer.Font.Color != "" {
			args = append(args, "-fill", layer.Font.Color, "-colorize", "100%")
		}
		if size != "" {
			args = append(args, "-resize", size)
		}
		args = append(args, tmpPath)
		if err := runCommand("magick", args...); err != nil {
			return fmt.Errorf("failed to render icon: %w", err)
		}
		mainElementPath = tmpPath

	} else if layer.Type == "box" {
		args := []string{"-size", fmt.Sprintf("%dx%d", layer.Width, layer.Height), "xc:none"}

		// NEW: Gradient Logic
		if layer.Box != nil && layer.Box.GradientStart != "" && layer.Box.GradientEnd != "" {
			angle := 90 // Default vertical
			if layer.Box.GradientAngle != 0 {
				angle = layer.Box.GradientAngle
			}
			args = append(args, "-define", fmt.Sprintf("gradient:angle=%d", angle))
			args = append(args, "-fill", fmt.Sprintf("gradient:%s-%s", layer.Box.GradientStart, layer.Box.GradientEnd))
		} else {
			// Fallback to layer.Fill (legacy) or layer.Box.Fill
			fill := layer.Fill
			if layer.Box != nil && layer.Box.Fill != "" {
				fill = layer.Box.Fill
			}
			args = append(args, "-fill", fill)
		}

		if layer.DrawPath != "" {
			pathData := strings.ReplaceAll(layer.DrawPath, "%w", strconv.Itoa(layer.Width))
			pathData = strings.ReplaceAll(pathData, "%h", strconv.Itoa(layer.Height))
			re := regexp.MustCompile(`(\d+)-(\d+)`)
			pathData = re.ReplaceAllStringFunc(pathData, func(s string) string {
				parts := strings.Split(s, "-")
				if len(parts) == 2 {
					val1, _ := strconv.Atoi(parts[0])
					val2, _ := strconv.Atoi(parts[1])
					return strconv.Itoa(val1 - val2)
				}
				return s
			})
			args = append(args, "-draw", fmt.Sprintf("path '%s'", pathData))
		} else {
			radius := layer.Radius
			if layer.Box != nil && layer.Box.Radius > 0 {
				radius = layer.Box.Radius
			}

			var drawCmd string
			if radius > 0 {
				drawCmd = fmt.Sprintf("roundrectangle 0,0 %d,%d %d,%d", layer.Width-1, layer.Height-1, radius, radius)
			} else {
				drawCmd = fmt.Sprintf("rectangle 0,0 %d,%d", layer.Width-1, layer.Height-1)
			}
			args = append(args, "-draw", drawCmd)
		}
		args = append(args, tmpPath)
		if err := runCommand("magick", args...); err != nil {
			return err
		}
		mainElementPath = tmpPath
	}

	if mainElementPath == "" {
		return fmt.Errorf("no element generated for layer %s", layer.Name)
	}

	if layer.Shadow != nil {
		shadowPath := filepath.Join(tmpDir, "shadow_"+layer.Name+".png")
		args := []string{mainElementPath, "(", "+clone", "-background", "black", "-shadow", fmt.Sprintf("%fx%d+%d+%d", layer.Shadow.Opacity*100, layer.Shadow.Blur, layer.Shadow.Dx, layer.Shadow.Dy), ")", "+swap", "-background", "none", "-layers", "merge", "+repage", shadowPath}
		if err := runCommand("magick", args...); err != nil {
			return err
		}
		mainElementPath = shadowPath
	}

	// Logic for Boxes behind Text (Text Layers)
	if (layer.Type == "text" || strings.HasPrefix(layer.Type, "text_")) && layer.Box != nil {
		boxedPath := filepath.Join(tmpDir, "boxed_"+layer.Name+".png")
		w, h, err := getPNGDimensions(mainElementPath)
		if err != nil {
			return err
		}
		boxW, boxH := w+(layer.Box.PaddingX*2), h+(layer.Box.PaddingY*2)
		boxLayerPath := filepath.Join(tmpDir, "boxbg_"+layer.Name+".png")

		boxArgs := []string{"-size", fmt.Sprintf("%dx%d", boxW, boxH), "xc:none"}

		// Gradient support for Text Boxes
		if layer.Box.GradientStart != "" && layer.Box.GradientEnd != "" {
			angle := 90
			if layer.Box.GradientAngle != 0 {
				angle = layer.Box.GradientAngle
			}
			boxArgs = append(boxArgs, "-define", fmt.Sprintf("gradient:angle=%d", angle))
			boxArgs = append(boxArgs, "-fill", fmt.Sprintf("gradient:%s-%s", layer.Box.GradientStart, layer.Box.GradientEnd))
		} else {
			boxArgs = append(boxArgs, "-fill", layer.Box.Fill)
		}

		if layer.Box.BorderWidth > 0 {
			boxArgs = append(boxArgs, "-stroke", layer.Box.BorderColor, "-strokewidth", strconv.Itoa(layer.Box.BorderWidth))
		}
		if layer.Box.Radius > 0 {
			boxArgs = append(boxArgs, "-draw", fmt.Sprintf("roundrectangle 0,0 %d,%d %d,%d", boxW-1, boxH-1, layer.Box.Radius, layer.Box.Radius), boxLayerPath)
		} else {
			boxArgs = append(boxArgs, "-draw", fmt.Sprintf("rectangle 0,0 %d,%d", boxW-1, boxH-1), boxLayerPath)
		}
		if err := runCommand("magick", boxArgs...); err != nil {
			return err
		}
		compositeArgs := []string{boxLayerPath, mainElementPath, "-gravity", "center", "-composite", boxedPath}
		if err := runCommand("magick", compositeArgs...); err != nil {
			return err
		}
		mainElementPath = boxedPath
	}

	// Always rename the final file
	return os.Rename(mainElementPath, outputPath)
}

func applyPostEffect(inputPath, effectName, tmpDir string, log zerolog.Logger) (string, error) {
	outputPath := filepath.Join(tmpDir, fmt.Sprintf("bg_effect_%s.mp4", effectName))
	var filter string
	switch effectName {
	case "film":
		filter = "noise=alls=15:allf=t+u,vignette=PI/5"
	case "warm":
		filter = "eq=contrast=1.1:saturation=1.3:gamma_r=1.1:gamma_b=0.9,vignette=PI/6"
	case "cool":
		filter = "eq=contrast=1.2:saturation=1.1:gamma_b=1.1:gamma_r=0.9:gamma_g=0.95"
	case "vintage":
		filter = "colorchannelmixer=.393:.769:.189:0:.349:.686:.168:0:.272:.534:.131,vignette=PI/4"
	default:
		return inputPath, nil
	}
	log.Info().Str("effect", effectName).Msg("Applying post-processing effect to background")
	args := []string{"-i", inputPath, "-vf", filter, "-c:v", "libx264", "-preset", "medium", "-crf", "22", "-y", outputPath}
	if err := runCommand("ffmpeg", args...); err != nil {
		return "", fmt.Errorf("failed to apply post effect '%s': %w", effectName, err)
	}
	return outputPath, nil
}
