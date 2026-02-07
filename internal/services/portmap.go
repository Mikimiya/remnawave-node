package services

import (
	"encoding/json"
	"fmt"
	"strconv"

	"go.uber.org/zap"
)

// ApplyPortMap replaces inbound ports in Xray config according to the port mapping.
// It modifies the "port" field of each inbound entry.
// Format of portMap: map[originalPort]mappedPort
func ApplyPortMap(configData map[string]interface{}, portMap map[int]int, logger *zap.Logger) map[string]interface{} {
	if len(portMap) == 0 {
		return configData
	}

	inbounds, ok := configData["inbounds"]
	if !ok {
		return configData
	}

	inboundList, ok := inbounds.([]interface{})
	if !ok {
		return configData
	}

	for i, inbound := range inboundList {
		inboundMap, ok := inbound.(map[string]interface{})
		if !ok {
			continue
		}

		portVal, exists := inboundMap["port"]
		if !exists {
			continue
		}

		originalPort := toInt(portVal)
		if originalPort == 0 {
			continue
		}

		if mappedPort, found := portMap[originalPort]; found {
			tag, _ := inboundMap["tag"].(string)
			logger.Info("Port mapping applied",
				zap.String("tag", tag),
				zap.Int("original", originalPort),
				zap.Int("mapped", mappedPort),
			)
			inboundList[i].(map[string]interface{})["port"] = mappedPort
		}
	}

	configData["inbounds"] = inboundList
	return configData
}

// ApplyPortMapToBytes is a convenience function that takes JSON bytes, applies port mapping,
// and returns the modified JSON bytes.
func ApplyPortMapToBytes(configBytes []byte, portMap map[int]int, logger *zap.Logger) ([]byte, error) {
	if len(portMap) == 0 {
		return configBytes, nil
	}

	var configData map[string]interface{}
	if err := json.Unmarshal(configBytes, &configData); err != nil {
		return nil, fmt.Errorf("failed to parse config for port mapping: %w", err)
	}

	configData = ApplyPortMap(configData, portMap, logger)

	result, err := json.Marshal(configData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config after port mapping: %w", err)
	}

	return result, nil
}

// toInt converts various numeric types to int
func toInt(val interface{}) int {
	switch v := val.(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
