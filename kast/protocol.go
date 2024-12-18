package kast

const DefaultReceiverID = "receiver-0"

// App IDs
const (
	DefaultMediaReceiverAppID     = "CC1AD845"
	DefaultMediaReceiverBetaAppID = "233637DE"
	YouTubeAppID                  = "233637DE"
	NetflixAppID                  = "CA5E8412"
)

type ReceiverStatusMsg struct {
	Type MsgType `json:"type"` // "RECEIVER_STATUS"

	Status ReceiverStatus `json:"status"`
}

type MediaStatusMsg struct {
	Type MsgType `json:"type"` // "MEDIA_STATUS"

	Status []MediaStatus `json:"status"`
}

// MediaStatus of the media artifact with respect to the session.
// See https://developers.google.com/cast/docs/media/messages#MediaStatus
type MediaStatus struct {
	// Unique ID for the playback of this specific session.
	// This ID is set by the receiver at LOAD and can be used to identify a specific instance of a playback.
	// For example, two playbacks of "Wish you were here" within the same session would each have a unique mediaSessionId.
	MediaSessionID int `json:"mediaSessionId"`

	// Unique ID for the currently playing media item.
	CurrentItemID int `json:"currentItemId"`

	// Position since the beginning of the content, in seconds.
	// For live stream - time in seconds from the beginning of the event that should be known to the player.
	CurrentTime float64 `json:"currentTime"`

	// List of media items in the session.
	Items []MediaItem      `json:"items"`
	Media MediaInformation `json:"media"`

	// Indicates whether the media time is progressing, and at what rate.
	// This is independent of the player state since the media time can stop in any state.
	// 1.0 is regular time, 0.5 is slow motion
	PlaybackRate float64 `json:"playbackRate"`

	// Describes the state of the player as one of the following:
	// - IDLE		Player has not been loaded yet
	// - PLAYING	Player is actively playing content
	// - BUFFERING	Player is in PLAY mode but not actively playing content (currentTime is not changing)
	// - PAUSED		Player is paused
	PlayerState PlayerState `json:"playerState"`

	// Indicates the repeat mode of the player.
	RepeatMode string `json:"repeatMode"`

	// Flags describing which media commands the media player supports:
	// - 1  Pause
	// - 2  Seek
	// - 4  Stream volume
	// - 8  Stream mute
	// - 16  Skip forward
	// - 32  Skip backward
	//
	// Combinations are described as summations; for example, Pause+Seek+StreamVolume+Mute == 15.
	SupportedMediaCommands MediaCommand `json:"supportedMediaCommands"`

	// Stream volume.
	Volume Volume `json:"volume"`
}

// PlayerState describes the state of the player as one of the following:
type PlayerState string

const (
	// Player has not been loaded yet
	PlayerStateIdle PlayerState = "IDLE"
	// Player is actively playing content
	PlayerStatePlaying PlayerState = "PLAYING"
	// Player is in PLAY mode but not actively playing content (currentTime is not changing)
	PlayerStateBuffering PlayerState = "BUFFERING"
	// Player is paused
	PlayerStatePaused PlayerState = "PAUSED"
)

// MediaCommand flags describing which media commands the media player supports.
type MediaCommand int

// All media commands.
const (
	MediaCommandPause MediaCommand = 1 << iota
	MediaCommandSeek
	MediaCommandStreamVolume
	MediaCommandStreamMute
	MediaCommandSkipForward
	MediaCommandSkipBackward
)

type MediaItem struct {
	ItemID  int              `json:"itemId"`
	Media   MediaInformation `json:"media"`
	OrderID int              `json:"orderId"`
}

type MediaInformation struct {
	// Service-specific identifier of the content currently loaded by the media player.
	// This is a free form string and is specific to the application.
	// In most cases, this will be the URL to the media, but the sender can choose to pass
	// a string that the receiver can interpret properly.
	//
	// Max length: 1024 characters.
	ContentID string `json:"contentId"`
	// MIME type of the content. Example: "video/mp4"
	ContentType string `json:"contentType"`
	// Type of the stream: "NONE", "BUFFERED", "LIVE"
	StreamType string `json:"streamType"`
	// (optional) Duration of the content in seconds.
	Duration float64 `json:"duration,omitempty"`
	// (optional) Metadata about the media content.
	Metadata *Metadata `json:"metadata,omitempty"`
}

type Metadata struct {
	// Type of the metadata.
	// 0 - GenericMediaMetadata
	// 1 - MovieMediaMetadata
	// 2 - TvShowMediaMetadata
	// 3 - MusicTrackMediaMetadata
	// 4 - PhotoMediaMetadata
	MetadataType int    `json:"metadataType"`
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
}

type ReceiverStatus struct {
	Applications  []Application `json:"applications,omitempty"`
	IsActiveInput bool          `json:"isActiveInput"`
	Volume        Volume        `json:"volume"`
}

type Application struct {
	AppID        string      `json:"appId"`       // e.g. "CC1AD845"
	DisplayName  string      `json:"displayName"` // e.g. "Default Media Receiver"
	IsIdleScreen bool        `json:"isIdleScreen"`
	Namespaces   []Namespace `json:"namespaces"`
	SessionID    string      `json:"sessionId"`   // e.g. "dafd3f7a-be22-11ef-ac28-32fe2a790a7e"
	StatusText   string      `json:"statusText"`  // e.g. "Ready To Cast"
	TransportID  string      `json:"transportId"` // e.g. "CastReceiver-81"
}

type Namespace struct {
	Name string `json:"name"` // e.g. "urn:x-cast:com.google.cast.media"
}

type Volume struct {
	// (optional) Current stream volume level as a value between 0.0 and 1.0 where 1.0 is the maximum volume.
	Level float64 `json:"level"`
	// (optional) Whether the Cast device is muted, independent of the volume level
	Muted bool `json:"muted"`

	ControlType  string  `json:"controlType"`  // e.g. "attenuation"
	StepInterval float64 `json:"stepInterval"` // e.g. 0.01
}
