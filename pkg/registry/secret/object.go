// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type Object struct {
	Auths map[string]*Auth `json:"auths"`
}

type Auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Auth     string `json:"auth"`
}

func (o *Object) Encode() string {
	// Make sure auth is set
	for _, auth := range o.Auths {
		a := fmt.Sprintf("%s:%s", auth.Username, auth.Password)
		auth.Auth = base64.StdEncoding.EncodeToString([]byte(a))
	}

	bytes, _ := json.Marshal(o)

	return base64.StdEncoding.EncodeToString(bytes)
}
