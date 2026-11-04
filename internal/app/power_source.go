package app

/*
#cgo LDFLAGS: -framework CoreFoundation -framework IOKit
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/ps/IOPowerSources.h>
#include <IOKit/ps/IOPSKeys.h>
#include <stdlib.h>
#include <string.h>

static int mactop_dict_number(CFDictionaryRef d, CFStringRef key, long long *out) {
    CFTypeRef v = CFDictionaryGetValue(d, key);
    if (!v || CFGetTypeID(v) != CFNumberGetTypeID())
        return 0;
    return CFNumberGetValue((CFNumberRef)v, kCFNumberLongLongType, out) ? 1 : 0;
}

static int mactop_dict_string(CFDictionaryRef d, CFStringRef key, char *out, int out_len) {
    CFTypeRef v = CFDictionaryGetValue(d, key);
    if (!v || CFGetTypeID(v) != CFStringGetTypeID() || out_len <= 0)
        return 0;
    out[0] = '\0';
    return CFStringGetCString((CFStringRef)v, out, out_len, kCFStringEncodingUTF8) ? 1 : 0;
}

static int mactop_read_source_type(char *out, int out_len) {
    if (!out || out_len <= 0)
        return 0;
    out[0] = '\0';
    CFTypeRef info = IOPSCopyPowerSourcesInfo();
    if (!info)
        return 0;
    CFStringRef providing = IOPSGetProvidingPowerSourceType(info);
    int ok = 0;
    if (providing) {
        ok = CFStringGetCString(providing, out, out_len, kCFStringEncodingUTF8) ? 1 : 0;
        CFRelease(providing);
    }
    CFRelease(info);
    return ok;
}

static int mactop_read_external_connected(void) {
    io_service_t bat = IOServiceGetMatchingService(kIOMainPortDefault,
                         IOServiceNameMatching("AppleSmartBattery"));
    if (!bat)
        return 0;
    CFTypeRef ext = IORegistryEntryCreateCFProperty(bat, CFSTR("ExternalConnected"),
                              kCFAllocatorDefault, 0);
    int connected = 0;
    if (ext && CFGetTypeID(ext) == CFBooleanGetTypeID())
        connected = CFBooleanGetValue((CFBooleanRef)ext) ? 1 : 0;
    if (ext)
        CFRelease(ext);
    IOObjectRelease(bat);
    return connected;
}

static void mactop_read_adapter(long long *watts, char *desc, int desc_len) {
    *watts = 0;
    if (desc && desc_len > 0)
        desc[0] = '\0';

    io_service_t bat = IOServiceGetMatchingService(kIOMainPortDefault,
                         IOServiceNameMatching("AppleSmartBattery"));
    if (!bat)
        return;

    CFTypeRef details = IORegistryEntryCreateCFProperty(bat, CFSTR("AdapterDetails"),
                              kCFAllocatorDefault, 0);
    if (details && CFGetTypeID(details) == CFDictionaryGetTypeID()) {
        long long w = 0;
        if (mactop_dict_number((CFDictionaryRef)details, CFSTR("Watts"), &w) && w > 0)
            *watts = w;
        mactop_dict_string((CFDictionaryRef)details, CFSTR("Description"), desc, desc_len);
    }
    if (details)
        CFRelease(details);
    IOObjectRelease(bat);
}
*/
import "C"

import (
	"strings"
	"sync"
	"time"
)

type PowerSupply struct {
	OnACPower bool `json:"on_ac_power" yaml:"on_ac_power" xml:"OnACPower" toon:"on_ac_power"`

	AdapterConnected bool `json:"adapter_connected" yaml:"adapter_connected" xml:"AdapterConnected" toon:"adapter_connected"`

	AdapterWatts int `json:"adapter_watts" yaml:"adapter_watts" xml:"AdapterWatts" toon:"adapter_watts"`

	AdapterDescription string `json:"adapter_description,omitempty" yaml:"adapter_description,omitempty" xml:"AdapterDescription,omitempty" toon:"adapter_description"`

	Source string `json:"source,omitempty" yaml:"source,omitempty" xml:"Source,omitempty" toon:"source"`
}

func (p PowerSupply) Rated() bool {
	return p.AdapterConnected && p.AdapterWatts > 0
}

var (
	adapterMu    sync.Mutex
	adapterWatts int
	adapterDesc  string
	adapterKnown bool
)

func readAdapterDetails() (int, string) {
	var watts C.longlong
	desc := make([]C.char, 64)
	C.mactop_read_adapter(&watts, &desc[0], C.int(len(desc)))
	return int(watts), C.GoString(&desc[0])
}

func cachedAdapter() (int, string, bool) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	return adapterWatts, adapterDesc, adapterKnown
}

func cacheAdapter(watts int, desc string) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	adapterWatts, adapterDesc, adapterKnown = watts, desc, true
}

var (
	powerSupplyMu     sync.Mutex
	powerSupplyCached PowerSupply
	powerSupplyAt     time.Time
	powerSupplyTTL    = 5 * time.Second
)

func readPowerSupplyUncached() PowerSupply {
	sourceBuf := make([]C.char, 64)
	source := ""
	if C.mactop_read_source_type(&sourceBuf[0], C.int(len(sourceBuf))) != 0 {
		source = C.GoString(&sourceBuf[0])
	}
	connected := C.mactop_read_external_connected() != 0

	watts, desc, known := cachedAdapter()
	if !known && connected {
		watts, desc = readAdapterDetails()
		if watts > 0 || desc != "" {
			cacheAdapter(watts, desc)
		}
	}

	supply := PowerSupply{
		OnACPower:          strings.EqualFold(source, "AC Power"),
		AdapterConnected:   connected,
		AdapterWatts:       watts,
		AdapterDescription: desc,
		Source:             source,
	}
	if !supply.OnACPower && supply.AdapterConnected {
		supply.OnACPower = true
	}
	return supply
}

func GetPowerSupply() PowerSupply {
	powerSupplyMu.Lock()
	defer powerSupplyMu.Unlock()
	if !powerSupplyAt.IsZero() && time.Since(powerSupplyAt) < powerSupplyTTL {
		return powerSupplyCached
	}
	powerSupplyCached = readPowerSupplyUncached()
	powerSupplyAt = time.Now()
	return powerSupplyCached
}
