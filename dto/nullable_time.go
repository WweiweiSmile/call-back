package dto

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// NullableTime 是一个可空的时间类型，可以处理空字符串的情况
type NullableTime struct {
	Time  time.Time
	Valid bool // Valid 为 true 表示 Time 有有效值
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (nt *NullableTime) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	if s == "" {
		nt.Valid = false
		return nil
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}

	nt.Time = t
	nt.Valid = true
	return nil
}

// MarshalJSON 实现 json.Marshaler 接口
func (nt NullableTime) MarshalJSON() ([]byte, error) {
	if !nt.Valid {
		return json.Marshal(nil)
	}
	return json.Marshal(nt.Time.Format(time.RFC3339))
}

// Value 实现 driver.Valuer 接口，用于数据库写入
func (nt NullableTime) Value() (driver.Value, error) {
	if !nt.Valid {
		return nil, nil
	}
	return nt.Time, nil
}

// Scan 实现 sql.Scanner 接口，用于数据库读取
func (nt *NullableTime) Scan(value interface{}) error {
	if value == nil {
		nt.Valid = false
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		nt.Time = v
		nt.Valid = true
		return nil
	default:
		return fmt.Errorf("无法将 %T 转换为 NullableTime", value)
	}
}

// ToTimePointer 将 NullableTime 转换为 *time.Time
func (nt *NullableTime) ToTimePointer() *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}

// FromTimePointer 从 *time.Time 创建 NullableTime
func FromTimePointer(t *time.Time) NullableTime {
	if t == nil {
		return NullableTime{Valid: false}
	}
	return NullableTime{Time: *t, Valid: true}
}
