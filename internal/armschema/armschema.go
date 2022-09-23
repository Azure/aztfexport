package armschema

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/magodo/azlist/azlist"
)

var once sync.Once
var armSchemas = map[string][]string{}

func GetARMSchemas() (map[string][]string, error) {
	var err error
	once.Do(func() {
		var m map[string][]string
		err = json.Unmarshal(azlist.ARMSchemaFile, &m)
		if err != nil {
			return
		}
		for k, v := range m {
			armSchemas[strings.ToUpper(k)] = v
		}
	})
	return armSchemas, nil

}
