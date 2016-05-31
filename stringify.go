package syncbox

import (
	"fmt"
	"reflect"
)

// ToString receives a pointer of structure and stringify it
func ToString(a interface{}) string {
	str := ""
	structType := reflect.TypeOf(a).Elem()
	structVal := reflect.ValueOf(a).Elem()
	for i := 0; i < structVal.NumField(); i++ {
		fieldType := structType.Field(i)
		fieldVal := structVal.Field(i)
		fieldName := fieldType.Name
		str += fieldName + ": "
		switch fieldVal.Kind() {
		case reflect.Slice:
			str += getSliceAndArrayString(fieldVal)
		case reflect.Array:
			str += getSliceAndArrayString(fieldVal)
		default:
			str += fmt.Sprintf("%v", fieldVal)
		}
		str += "\n"
	}
	return str
}

func getSliceAndArrayString(field reflect.Value) string {
	str := ""
	if field.Len() > 0 {
		switch field.Type() {
		case reflect.TypeOf([]byte(nil)):
			str += string(field.Bytes())
		default:
			str += fmt.Sprintf("%v", field)
		}
	}
	return str
}
