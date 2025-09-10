/*
Copyright 2025.

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

package confighandler

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type ConfigField struct {
	FieldName  string   // Go struct field name (e.g., "ServiceAccountName")
	ConfigKey  string   // Config key for YAML/Viper (e.g., "serviceAccountName")
	EnvVar     string   // Environment variable name (e.g., "TAGEMON_SERVICEACCOUNTNAME")
	TestValues []string // Test values to use
}

var testableFields = []ConfigField{
	{
		FieldName: "ServiceAccountName",
		ConfigKey: "serviceAccountName",
		EnvVar:    "TAGEMON_SERVICEACCOUNTNAME",
		TestValues: []string{
			"test-service-account",
			"sa-with-special-chars_123.test",
			"  sa-with-spaces  ",
			strings.Repeat("a", 100), // Long value
			"",                       // Empty value
		},
	},
}

func TestConfig(t *testing.T) {
	t.Run("struct initialization", func(t *testing.T) {
		for _, field := range testableFields {
			t.Run(field.FieldName, func(t *testing.T) {
				config := Config{}

				// Use reflection to set and verify field
				configValue := reflect.ValueOf(&config).Elem()
				fieldValue := configValue.FieldByName(field.FieldName)

				if !fieldValue.IsValid() {
					t.Fatalf("Field %s not found in Config struct", field.FieldName)
				}

				// Test setting a value
				if fieldValue.Kind() == reflect.String {
					fieldValue.SetString(field.TestValues[0])
					if fieldValue.String() != field.TestValues[0] {
						t.Errorf("Expected %s to be '%s', got '%s'", field.FieldName, field.TestValues[0], fieldValue.String())
					}
				}

				// Test empty value
				fieldValue.SetString("")
				if fieldValue.String() != "" {
					t.Errorf("Expected %s to be empty, got '%s'", field.FieldName, fieldValue.String())
				}
			})
		}
	})
}

//nolint:gocyclo // Test function complexity is acceptable for comprehensive testing
func TestLoadConfig(t *testing.T) {
	setupTest := func(t *testing.T) (string, string, func()) {
		tempDir, err := os.MkdirTemp("", "confighandler-test")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}

		configPath := filepath.Join(tempDir, "config")
		err = os.MkdirAll(configPath, 0755)
		if err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}

		originalDir, _ := os.Getwd()
		err = os.Chdir(tempDir)
		if err != nil {
			t.Fatalf("failed to change directory: %v", err)
		}

		clearEnvVars()

		cleanup := func() {
			clearEnvVars()
			_ = os.Chdir(originalDir)
			_ = os.RemoveAll(tempDir)
		}

		t.Cleanup(cleanup)
		return tempDir, configPath, cleanup
	}

	getConfigFieldValue := func(config *Config, fieldName string) string {
		configValue := reflect.ValueOf(config).Elem()
		fieldValue := configValue.FieldByName(fieldName)
		if !fieldValue.IsValid() || fieldValue.Kind() != reflect.String {
			return ""
		}
		return fieldValue.String()
	}

	t.Run("no config file and no environment variables", func(t *testing.T) {
		_, _, _ = setupTest(t)

		config := LoadConfig()

		if config == nil {
			t.Fatal("expected config to not be nil")
		}

		for _, field := range testableFields {
			value := getConfigFieldValue(config, field.FieldName)
			if value != "" {
				t.Errorf("expected %s to be empty, got '%s'", field.FieldName, value)
			}
		}
	})

	t.Run("loading from config file", func(t *testing.T) {
		field := testableFields[0]

		t.Run("should load valid YAML config from ./config directory", func(t *testing.T) {
			_, configPath, _ := setupTest(t)

			configFile := filepath.Join(configPath, "controller-config.yaml")
			configContent := fmt.Sprintf(`%s: "%s"`, field.ConfigKey, field.TestValues[0])

			err := os.WriteFile(configFile, []byte(configContent), 0644)
			if err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			config := LoadConfig()

			if config == nil {
				t.Fatal("expected config to not be nil")
			}
			value := getConfigFieldValue(config, field.FieldName)
			if value != field.TestValues[0] {
				t.Errorf("expected %s to be '%s', got '%s'", field.FieldName, field.TestValues[0], value)
			}
		})

		t.Run("error handling", func(t *testing.T) {
			t.Run("should panic on malformed YAML", func(t *testing.T) {
				_, configPath, _ := setupTest(t)

				configFile := filepath.Join(configPath, "controller-config.yaml")
				// Create truly malformed YAML that will definitely cause parse error
				malformedContent := `
serviceAccountName: "test-value"
[invalid yaml structure with unclosed brackets and bad indentation
  - item1:
	bad indentation
    missing closing bracket
`

				err := os.WriteFile(configFile, []byte(malformedContent), 0644)
				if err != nil {
					t.Fatalf("failed to write malformed config: %v", err)
				}

				// Test that LoadConfig panics with malformed YAML
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected LoadConfig to panic on malformed YAML, but it didn't")
					} else {
						// Verify panic message contains expected info
						panicMsg := fmt.Sprintf("%v", r)
						if !strings.Contains(panicMsg, "Invalid configuration file") {
							t.Errorf("expected panic message to contain 'Invalid configuration file', got: %s", panicMsg)
						}
					}
				}()

				LoadConfig()
			})

			t.Run("should handle empty YAML file", func(t *testing.T) {
				_, configPath, _ := setupTest(t)

				configFile := filepath.Join(configPath, "controller-config.yaml")
				err := os.WriteFile(configFile, []byte(""), 0644)
				if err != nil {
					t.Fatalf("failed to write empty config: %v", err)
				}

				config := LoadConfig()

				if config == nil {
					t.Fatal("expected config to not be nil")
				}
				value := getConfigFieldValue(config, field.FieldName)
				if value != "" {
					t.Errorf("expected %s to be empty for empty file, got '%s'", field.FieldName, value)
				}
			})
		})
	})

	t.Run("loading from environment variables", func(t *testing.T) {
		// Test all fields with their environment variables
		for _, field := range testableFields {
			t.Run(field.FieldName, func(t *testing.T) {
				for i, testValue := range field.TestValues {
					if testValue == "" {
						continue // Skip empty test value for this test
					}

					t.Run(fmt.Sprintf("value_%d", i), func(t *testing.T) {
						_, _, _ = setupTest(t)

						_ = os.Setenv(field.EnvVar, testValue)

						config := LoadConfig()

						if config == nil {
							t.Fatal("expected config to not be nil")
						}
						value := getConfigFieldValue(config, field.FieldName)
						if value != testValue {
							t.Errorf("expected %s to be '%s', got '%s'", field.FieldName, testValue, value)
						}
					})
				}
			})
		}

		t.Run("should handle empty environment variable", func(t *testing.T) {
			field := testableFields[0]
			_, _, _ = setupTest(t)

			_ = os.Setenv(field.EnvVar, "")

			config := LoadConfig()

			if config == nil {
				t.Fatal("expected config to not be nil")
			}
			value := getConfigFieldValue(config, field.FieldName)
			if value != "" {
				t.Errorf("expected %s to be empty, got '%s'", field.FieldName, value)
			}
		})
	})

	t.Run("precedence rules", func(t *testing.T) {
		field := testableFields[0]

		t.Run("should prefer environment variables over file config", func(t *testing.T) {
			_, configPath, _ := setupTest(t)

			// Create config file
			configFile := filepath.Join(configPath, "controller-config.yaml")
			configContent := fmt.Sprintf(`%s: "file-value"`, field.ConfigKey)

			err := os.WriteFile(configFile, []byte(configContent), 0644)
			if err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			// Set environment variable
			_ = os.Setenv(field.EnvVar, "env-value")

			config := LoadConfig()

			if config == nil {
				t.Fatal("expected config to not be nil")
			}
			value := getConfigFieldValue(config, field.FieldName)
			if value != "env-value" {
				t.Errorf("expected %s to be 'env-value', got '%s'", field.FieldName, value)
			}
		})

		t.Run("should use file config when environment variable is empty", func(t *testing.T) {
			_, configPath, _ := setupTest(t)

			// Create config file
			configFile := filepath.Join(configPath, "controller-config.yaml")
			configContent := fmt.Sprintf(`%s: "file-value"`, field.ConfigKey)

			err := os.WriteFile(configFile, []byte(configContent), 0644)
			if err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			// Set empty environment variable
			_ = os.Setenv(field.EnvVar, "")

			config := LoadConfig()

			if config == nil {
				t.Fatal("expected config to not be nil")
			}
			value := getConfigFieldValue(config, field.FieldName)
			if value != "file-value" {
				t.Errorf("expected %s to be 'file-value', got '%s'", field.FieldName, value)
			}
		})
	})

	t.Run("concurrent access", func(t *testing.T) {
		t.Run("should handle multiple LoadConfig calls safely", func(t *testing.T) {
			field := testableFields[0]
			_, configPath, _ := setupTest(t)

			// Set up config
			configFile := filepath.Join(configPath, "controller-config.yaml")
			configContent := fmt.Sprintf(`%s: "concurrent-test"`, field.ConfigKey)

			err := os.WriteFile(configFile, []byte(configContent), 0644)
			if err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			// Load config multiple times concurrently
			const numGoroutines = 10
			results := make(chan *Config, numGoroutines)
			var wg sync.WaitGroup

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results <- LoadConfig()
				}()
			}

			wg.Wait()
			close(results)

			// Collect and verify results
			count := 0
			for config := range results {
				count++
				if config == nil {
					t.Error("expected config to not be nil")
					continue
				}
				value := getConfigFieldValue(config, field.FieldName)
				if value != "concurrent-test" {
					t.Errorf("expected %s to be 'concurrent-test', got '%s'", field.FieldName, value)
				}
			}

			if count != numGoroutines {
				t.Errorf("expected %d results, got %d", numGoroutines, count)
			}
		})
	})
}

// Helper function to clear environment variables with TAGEMON prefix
func clearEnvVars() {
	envVars := os.Environ()
	for _, envVar := range envVars {
		if strings.HasPrefix(envVar, "TAGEMON_") {
			key := strings.Split(envVar, "=")[0]
			_ = os.Unsetenv(key)
		}
	}
}
