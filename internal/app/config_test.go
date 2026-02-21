/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultOperatorConfig(t *testing.T) {
	cfg := DefaultOperatorConfig()

	assert.Equal(t, 8*time.Hour, cfg.DefaultDriftInterval)
	assert.Equal(t, "default", cfg.InstanceID)
}

func TestOperatorConfigZeroValue(t *testing.T) {
	var cfg OperatorConfig

	assert.Equal(t, time.Duration(0), cfg.DefaultDriftInterval)
	assert.Equal(t, "", cfg.InstanceID)
}
