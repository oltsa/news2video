package main

import "encoding/json"

type RenderJob struct {
	JobID            string                 `json:"job_id"`
	CorrelationID    string                 `json:"correlation_id,omitempty"`
	TemplateID       string                 `json:"template_id,omitempty"`
	Payload          map[string]interface{} `json:"payload"`
	Template         json.RawMessage        `json:"template"`
	OutputStorageKey string                 `json:"output_storage_key"`
	Destinations     json.RawMessage        `json:"destinations,omitempty"`
}

type AudioTrack struct {
	File       string  `json:"file"`
	Loop       bool    `json:"loop,omitempty"`
	StartAtSec float64 `json:"start_at_sec,omitempty"`
	Volume     float64 `json:"volume,omitempty"`
}

type Template struct {
	Name          string       `json:"name"`
	SchemaVersion string       `json:"schema_version"`
	Canvas        Canvas       `json:"canvas"`
	DurationSec   float64      `json:"duration_sec"`
	FPS           int          `json:"fps"`
	Layers        []Layer      `json:"layers"`
	AudioTracks   []AudioTrack `json:"audio_tracks,omitempty"`
}

type BoundingBox struct {
	Width  int `json:"width"`
	Height int `json:"height,omitempty"`
}

type Font struct {
	File  string `json:"file,omitempty"`
	Size  int    `json:"size,omitempty"`
	Color string `json:"color,omitempty"`
	Align string `json:"align,omitempty"`
}

type Layer struct {
	Name        string       `json:"name"`
	Type        string       `json:"type"`
	File        string       `json:"file,omitempty"`
	TextContent string       `json:"text_content,omitempty"`
	PostEffect  string       `json:"post_effect,omitempty"`
	StartSec    float64      `json:"start_sec"`
	EndSec      float64      `json:"end_sec"`
	Pos         Pos          `json:"pos"`
	Gravity     string       `json:"gravity"`
	Sizing      *Sizing      `json:"sizing,omitempty"`
	Motion      *Motion      `json:"motion,omitempty"`
	Filters     *Filters     `json:"filters,omitempty"`
	Animations  []Animation  `json:"animations,omitempty"`
	Width       int          `json:"width,omitempty"`
	Height      int          `json:"height,omitempty"`
	Fill        string       `json:"fill,omitempty"`
	DrawPath    string       `json:"draw_path,omitempty"`
	Radius      int          `json:"radius,omitempty"`
	Shadow      *Shadow      `json:"shadow,omitempty"`
	Box         *Box         `json:"box,omitempty"`
	BoundingBox *BoundingBox `json:"boundingBox,omitempty"`
	Font        *Font        `json:"font,omitempty"`
}

type Canvas struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Sizing struct {
	Mode   string `json:"mode"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type Filters map[string]interface{}

type Animation struct {
	Type        string  `json:"type"`
	StartSec    float64 `json:"start_sec,omitempty"`
	DurationSec float64 `json:"duration_sec"`
	Direction   string  `json:"direction,omitempty"`
	TargetX     *int    `json:"target_x,omitempty"`
	TargetY     *int    `json:"target_y,omitempty"`
	Distance    *int    `json:"distance,omitempty"`
	Easing      string  `json:"easing,omitempty"`
}

type ZoomTarget struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Motion struct {
	ZoomStart float64    `json:"zoom_start"`
	ZoomEnd   float64    `json:"zoom_end"`
	PanStart  ZoomTarget `json:"pan_start"`
	PanEnd    ZoomTarget `json:"pan_end"`
	Type      string     `json:"type,omitempty"`
	Speed     string     `json:"speed,omitempty"`
	ZoomRange []float64  `json:"zoom_range,omitempty"`
	PanXRange []float64  `json:"pan_x_range,omitempty"`
	PanYRange []float64  `json:"pan_y_range,omitempty"`
}

type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Shadow struct {
	Opacity float64 `json:"opacity"`
	Blur    int     `json:"blur"`
	Dx      int     `json:"dx"`
	Dy      int     `json:"dy"`
}

type Box struct {
	PaddingX      int    `json:"padding_x"`
	PaddingY      int    `json:"padding_y"`
	Radius        int    `json:"radius"`
	Fill          string `json:"fill,omitempty"`
	BorderColor   string `json:"border_color,omitempty"`
	BorderWidth   int    `json:"border_width,omitempty"`
	GradientStart string `json:"gradient_start,omitempty"`
	GradientEnd   string `json:"gradient_end,omitempty"`
	GradientAngle int    `json:"gradient_angle,omitempty"`
}
