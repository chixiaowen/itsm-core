package catalog

import (
	"encoding/json"
	"strings"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
)

// 动态表单支持的字段类型（对齐 PRD REQ-CAT-006 / ARCHITECTURE §14）。
const (
	// FieldTypeText 单行文本。
	FieldTypeText = "text"
	// FieldTypeNumber 数字。
	FieldTypeNumber = "number"
	// FieldTypeSelect 下拉选择。
	FieldTypeSelect = "select"
	// FieldTypeDate 日期。
	FieldTypeDate = "date"
	// FieldTypeTextarea 多行文本。
	FieldTypeTextarea = "textarea"
)

// FormField 描述一个动态表单字段。
type FormField struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

// FormSchema 是服务项表单定义（存储为 form_schema JSON 字符串）。
type FormSchema struct {
	Fields []FormField `json:"fields"`
}

// validFieldType 判断字段类型是否受支持。
func validFieldType(t string) bool {
	switch t {
	case FieldTypeText, FieldTypeNumber, FieldTypeSelect, FieldTypeDate, FieldTypeTextarea:
		return true
	default:
		return false
	}
}

// ParseFormSchema 解析表单定义；空字符串视为「无字段」的空定义。
//
// 解析失败或字段定义非法返回 400（httpx.ErrBadRequest）。
func ParseFormSchema(raw string) (*FormSchema, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return &FormSchema{Fields: []FormField{}}, nil
	}
	var fs FormSchema
	if err := json.Unmarshal([]byte(raw), &fs); err != nil {
		return nil, httpx.ErrBadRequest("form_schema 不是合法 JSON: " + err.Error())
	}
	for i := range fs.Fields {
		f := &fs.Fields[i]
		if strings.TrimSpace(f.Name) == "" {
			return nil, httpx.ErrBadRequest("form_schema 字段缺少 name")
		}
		if !validFieldType(f.Type) {
			return nil, httpx.ErrBadRequest("不支持的字段类型: " + f.Type)
		}
		if f.Type == FieldTypeSelect && len(f.Options) == 0 {
			return nil, httpx.ErrBadRequest("select 字段必须提供 options: " + f.Name)
		}
	}
	return &fs, nil
}

// ParseFormSchemaStrict 解析表单定义，且要求非空（用于发布前置校验）。
func ParseFormSchemaStrict(raw string) (*FormSchema, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, httpx.ErrBadRequest("form_schema 不能为空")
	}
	return ParseFormSchema(raw)
}

// Validate 校验用户提交的动态表单数据：
// 必填字段缺失/为空返回 400；类型不匹配返回 400；select 取值不在 options 内返回 400。
func (fs *FormSchema) Validate(data map[string]any) error {
	if fs == nil {
		return nil
	}
	for _, f := range fs.Fields {
		v, ok := data[f.Name]
		if !ok || isEmptyValue(v) {
			if f.Required {
				return httpx.ErrBadRequest("缺少必填字段: " + fieldLabel(f))
			}
			continue
		}
		if err := validateFieldValue(f, v); err != nil {
			return err
		}
	}
	return nil
}

// fieldLabel 返回字段展示名（优先 Label，回退 Name）。
func fieldLabel(f FormField) string {
	if strings.TrimSpace(f.Label) != "" {
		return f.Label
	}
	return f.Name
}

// isEmptyValue 判断动态表单值是否为空。
func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	default:
		return false
	}
}

// validateFieldValue 校验单个字段取值类型。
func validateFieldValue(f FormField, v any) error {
	switch f.Type {
	case FieldTypeNumber:
		switch v.(type) {
		case float64, float32, int, int64, json.Number:
			return nil
		default:
			return httpx.ErrBadRequest("字段类型应为数字: " + fieldLabel(f))
		}
	case FieldTypeSelect:
		s, ok := v.(string)
		if !ok {
			return httpx.ErrBadRequest("字段类型应为文本: " + fieldLabel(f))
		}
		for _, opt := range f.Options {
			if opt == s {
				return nil
			}
		}
		return httpx.ErrBadRequest("字段取值不在选项内: " + fieldLabel(f))
	case FieldTypeText, FieldTypeTextarea, FieldTypeDate:
		if _, ok := v.(string); !ok {
			return httpx.ErrBadRequest("字段类型应为文本: " + fieldLabel(f))
		}
		return nil
	default:
		return nil
	}
}
