package attachment

type Metadata struct {
	ID             string `json:"id"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type"`
	Bytes          int64  `json:"bytes"`
	SHA256         string `json:"sha256"`
	ContentRef     string `json:"content_ref"`
	Active         bool   `json:"active"`
	Revision       uint64 `json:"revision"`
	RemoveClientID string `json:"-"`
	CreatedBy      string `json:"created_by"`
	DeviceID       string `json:"device_id"`
	RemoveDeviceID string `json:"remove_device_id,omitempty"`
	CreatedAt      int64  `json:"created_at"`
}

type Download struct {
	Attachment Metadata `json:"attachment"`
	Data       string   `json:"data"`
}
