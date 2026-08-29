package model

// P987Message mirrors TCM-26's Gr26Message so shelter's claim/upload path
// (and Mapache's parquet replay) work against the same column set.
// SourceNode carries the bus label; TargetNode is unused on stock Porsche
// CAN and kept only for schema compatibility.
type P987Message struct {
	Timestamp  int    `json:"timestamp" gorm:"index:p987_message_unsynced_ts,where:synced = 0"`
	VehicleID  string `json:"vehicle_id"`
	Topic      string `json:"topic"`
	Data       []byte `json:"data" gorm:"type:blob"`
	Synced     int    `json:"synced"`
	SourceNode string `json:"source_node"`
	TargetNode string `json:"target_node"`
}

func (P987Message) TableName() string {
	return "p987_message"
}
