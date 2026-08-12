package adb

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// DeviceState represents the state adb reports for a connected device.
type DeviceState string

const (
	StateUnknown      DeviceState = "unknown"
	StateDevice       DeviceState = "device"       // online & ready
	StateOffline      DeviceState = "offline"
	StateUnauthorized DeviceState = "unauthorized" // adb authorization pending
	StateBootloader   DeviceState = "bootloader"
	StateRecovery     DeviceState = "recovery"
	StateConnecting   DeviceState = "connecting"
	StateNoPermissions DeviceState = "no permissions"
)

// Device is a snapshot of a connected Android device as reported by
// `adb devices -l`. It is a value object — callers should not mutate it.
type Device struct {
	Serial       string
	State        DeviceState
	Product      string
	Model        string
	Device       string
	TransportID  string
	Manufacturer string
	// Info holds any additional key:value pairs from `adb devices -l` that
	// we don't parse into named fields (future-compatibility).
	Info map[string]string
}

// DisplayName returns a human-friendly single-line label suitable for a
// device-select dropdown: "<Model> (<Serial>)". Unknown models fall back to
// the serial number.
func (d Device) DisplayName() string {
	model := strings.TrimSpace(d.Model)
	if model == "" {
		model = strings.TrimSpace(d.Product)
	}
	if model == "" {
		return d.Serial
	}
	return fmt.Sprintf("%s (%s)", strings.ReplaceAll(model, "_", " "), d.Serial)
}

// StableID returns a stable identifier for the device — its serial.
func (d Device) StableID() string { return d.Serial }

// ListDevices runs `adb devices -l` and parses the output into a slice of
// Device structs. Devices in `offline`, `unauthorized`, or other non-ready
// states are returned so that callers can surface appropriate UI; check
// Device.State to decide whether a device is usable.
func (c *Client) ListDevices(ctx context.Context) ([]Device, error) {
	out, err := c.ServerCmd(ctx, "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	return parseDevicesList(out), nil
}

// WaitForDevice blocks until the given serial reaches the "device" (online)
// state, or until ctx is canceled. It simply shells out to
// `adb -s <serial> wait-for-any-device`? We use `wait-for-device` which is
// per-serial when `-s` is passed.
func (c *Client) WaitForDevice(ctx context.Context, serial string) error {
	if serial == "" {
		return errors.New("adb: wait requires a serial")
	}
	_, err := c.Command(ctx, serial, "wait-for-device")
	return err
}

// GetProps fetches all system properties from the device via `getprop` and
// returns them as a map. Useful for extracting manufacturer, model, SDK
// level, etc. Returns a partial map plus error if getprop fails on a single
// device.
func (c *Client) GetProps(ctx context.Context, serial string) (map[string]string, error) {
	out, err := c.Command(ctx, serial, "shell", "getprop")
	if err != nil {
		// Some older devices use `getprop` differently; try a single-line fallback
		return nil, fmt.Errorf("getprop: %w", err)
	}
	props := make(map[string]string)
	// adb shell getprop emits lines like `[ro.product.model]: [Pixel 7]`
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip surrounding brackets
		// Format: [key]: [value]
		// Be tolerant: strip the leading `[`, split on `]: [`, strip trailing `]`
		if !strings.HasPrefix(line, "[") {
			continue
		}
		sep := "]: ["
		idx := strings.Index(line, sep)
		if idx < 0 {
			continue
		}
		key := strings.TrimPrefix(line[:idx], "[")
		val := line[idx+len(sep):]
		val = strings.TrimSuffix(val, "]")
		props[key] = val
	}
	return props, nil
}

// DeviceInfo is a higher-level, convenience view of a device assembled from
// both `adb devices -l` and `getprop` output. All fields are best-effort.
type DeviceInfo struct {
	Device
	Manufacturer string
	Brand        string
	Model        string
	AndroidVer   string
	SDK          string
	BuildID      string
	SerialNo     string // ro.serialno (when different from transport serial)
}

// InspectDevice retrieves rich metadata for the given serial. Properties that
// can't be fetched (e.g. unauthorized device) are silently left empty so that
// list views can still show the Device snapshot.
func (c *Client) InspectDevice(ctx context.Context, d Device) DeviceInfo {
	info := DeviceInfo{Device: d}
	info.Model = d.Model
	info.Manufacturer = d.Manufacturer
	if d.State != StateDevice {
		return info
	}
	props, err := c.GetProps(ctx, d.Serial)
	if err != nil {
		return info
	}
	if v := props["ro.product.manufacturer"]; v != "" {
		info.Manufacturer = v
	}
	if v := props["ro.product.brand"]; v != "" {
		info.Brand = v
	}
	if v := props["ro.product.model"]; v != "" {
		info.Model = v
	}
	if v := props["ro.build.version.release"]; v != "" {
		info.AndroidVer = v
	}
	if v := props["ro.build.version.sdk"]; v != "" {
		info.SDK = v
	}
	if v := props["ro.build.display.id"]; v != "" {
		info.BuildID = v
	}
	if v := props["ro.serialno"]; v != "" {
		info.SerialNo = v
	}
	return info
}

// parseDevicesList parses the output of `adb devices -l`.
//
// Example output:
//
//	List of devices attached
//	emulator-5554    device product:sdk_gphone64_arm64 model:sdk_gphone64_arm64 device:emu64a transport_id:14
//	XXXXXXXXXXX     unauthorized usb:1-2
func parseDevicesList(out string) []Device {
	var devices []Device
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "list of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{
			Serial: fields[0],
			State:  DeviceState(fields[1]),
			Info:   map[string]string{},
		}
		for _, kv := range fields[2:] {
			if i := strings.IndexByte(kv, ':'); i > 0 {
				k := kv[:i]
				v := kv[i+1:]
				d.Info[k] = v
				switch k {
				case "product":
					d.Product = v
				case "model":
					d.Model = v
				case "device":
					d.Device = v
				case "transport_id":
					d.TransportID = v
				}
			}
		}
		// usb:<path> entries don't have key:value; ignore.
		devices = append(devices, d)
	}
	return devices
}
