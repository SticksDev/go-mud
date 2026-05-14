//go:build windows

package audio

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	coinitApartmentThreaded = 0x2
	clsctxAll               = 0x17
	eRender                 = 0
	eConsole                = 0
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// CLSIDs and IIDs for WASAPI audio session metering.
var (
	clsidMMDeviceEnumerator   = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator    = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioSessionManager2  = guid{0x77AA99A0, 0x1BD6, 0x484F, [8]byte{0x8B, 0xC7, 0x2C, 0x65, 0x4C, 0x9A, 0x9B, 0x6F}}
	iidIAudioSessionControl2  = guid{0xBFB7FF88, 0x7239, 0x4FC9, [8]byte{0x8F, 0xA2, 0x07, 0xC9, 0x50, 0xBE, 0x9C, 0x6D}}
	iidIAudioMeterInformation = guid{0xC02216F6, 0x8C67, 0x4B5B, [8]byte{0x9D, 0x00, 0xD0, 0x08, 0xE7, 0x3E, 0x00, 0x64}}
)

//
// Each COM interface inherits from IUnknown (QueryInterface=0, AddRef=1, Release=2).
// Indexes below are the method's position in the vtable.

const (
	// IUnknown
	iunknownQueryInterface = 0
	iunknownRelease        = 2

	// IMMDeviceEnumerator vtable:
	//   0 QueryInterface  1 AddRef  2 Release
	//   3 EnumAudioEndpoints  4 GetDefaultAudioEndpoint
	immDeviceEnumGetDefaultEndpoint = 4

	// IMMDevice vtable:
	//   0 QueryInterface  1 AddRef  2 Release
	//   3 Activate  4 OpenPropertyStore  5 GetId  6 GetState
	immDeviceActivate = 3

	// IAudioSessionManager2 vtable (inherits IAudioSessionManager):
	//   0-2 IUnknown  3 GetAudioSessionControl  4 GetSimpleAudioVolume
	//   5 GetSessionEnumerator  6-9 Register/Unregister notifications
	iasm2GetSessionEnumerator = 5

	// IAudioSessionEnumerator vtable:
	//   0-2 IUnknown  3 GetCount  4 GetSession
	iaseGetCount   = 3
	iaseGetSession = 4

	// IAudioSessionControl2 vtable (inherits IAudioSessionControl):
	//   0-2 IUnknown  3-11 IAudioSessionControl methods
	//   12 GetSessionIdentifier  13 GetSessionInstanceIdentifier
	//   14 GetProcessId  15 IsSystemSoundsSession  16 SetDuckingPreference
	iasc2GetProcessId = 14

	// IAudioMeterInformation vtable:
	//   0-2 IUnknown  3 GetPeakValue  4 GetMeteringChannelCount
	//   5 GetChannelsPeakValues  6 QueryHardwareSupport
	iamGetPeakValue = 3
)

var (
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	coInitializeEx   = ole32.NewProc("CoInitializeEx")
	coUninitializeFn = ole32.NewProc("CoUninitialize")
	coCreateInstance = ole32.NewProc("CoCreateInstance")

	user32                   = windows.NewLazySystemDLL("user32.dll")
	findWindowW              = user32.NewProc("FindWindowW")
	getWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

// comCall calls a method at the given vtable index on a COM object.
func comCall(obj uintptr, method uintptr, args ...uintptr) uintptr {
	vtable := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtable + method*unsafe.Sizeof(uintptr(0))))
	ret, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return ret
}

func checkHR(hr uintptr, op string) error {
	if hr != 0 {
		return fmt.Errorf("%s: HRESULT %#x", op, hr)
	}
	return nil
}

type comObject struct{ ptr uintptr }

func (o comObject) Release() {
	if o.ptr != 0 {
		comCall(o.ptr, iunknownRelease)
	}
}

func (o comObject) QueryInterface(iid *guid) (comObject, error) {
	var result uintptr
	hr := comCall(o.ptr, iunknownQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&result)))
	if err := checkHR(hr, "QueryInterface"); err != nil {
		return comObject{}, err
	}
	return comObject{result}, nil
}

type iMMDeviceEnumerator struct{ comObject }
type iMMDevice struct{ comObject }
type iAudioSessionManager2 struct{ comObject }
type iAudioSessionEnumerator struct{ comObject }
type iAudioSessionControl struct{ comObject }
type iAudioSessionControl2 struct{ comObject }
type iAudioMeterInformation struct{ comObject }

func (e iMMDeviceEnumerator) GetDefaultAudioEndpoint() (iMMDevice, error) {
	var dev uintptr
	hr := comCall(e.ptr, immDeviceEnumGetDefaultEndpoint, eRender, eConsole, uintptr(unsafe.Pointer(&dev)))
	if err := checkHR(hr, "GetDefaultAudioEndpoint"); err != nil {
		return iMMDevice{}, err
	}
	return iMMDevice{comObject{dev}}, nil
}

func (d iMMDevice) Activate(iid *guid) (comObject, error) {
	var out uintptr
	hr := comCall(d.ptr, immDeviceActivate, uintptr(unsafe.Pointer(iid)), clsctxAll, 0, uintptr(unsafe.Pointer(&out)))
	if err := checkHR(hr, "IMMDevice.Activate"); err != nil {
		return comObject{}, err
	}
	return comObject{out}, nil
}

func (m iAudioSessionManager2) GetSessionEnumerator() (iAudioSessionEnumerator, error) {
	var out uintptr
	hr := comCall(m.ptr, iasm2GetSessionEnumerator, uintptr(unsafe.Pointer(&out)))
	if err := checkHR(hr, "GetSessionEnumerator"); err != nil {
		return iAudioSessionEnumerator{}, err
	}
	return iAudioSessionEnumerator{comObject{out}}, nil
}

func (e iAudioSessionEnumerator) GetCount() int32 {
	var count int32
	comCall(e.ptr, iaseGetCount, uintptr(unsafe.Pointer(&count)))
	return count
}

func (e iAudioSessionEnumerator) GetSession(i int32) (iAudioSessionControl, error) {
	var out uintptr
	hr := comCall(e.ptr, iaseGetSession, uintptr(i), uintptr(unsafe.Pointer(&out)))
	if err := checkHR(hr, "GetSession"); err != nil {
		return iAudioSessionControl{}, err
	}
	return iAudioSessionControl{comObject{out}}, nil
}

func (c iAudioSessionControl2) GetProcessId() uint32 {
	var pid uint32
	comCall(c.ptr, iasc2GetProcessId, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func (m iAudioMeterInformation) GetPeakValue() (float32, error) {
	var peak float32
	hr := comCall(m.ptr, iamGetPeakValue, uintptr(unsafe.Pointer(&peak)))
	if err := checkHR(hr, "GetPeakValue"); err != nil {
		return 0, err
	}
	return peak, nil
}

type comChain struct {
	enumerator iMMDeviceEnumerator
	device     iMMDevice
	sessionMgr iAudioSessionManager2

	// Cached session meter for the target PID.
	cachedPID   uint32
	cachedMeter iAudioMeterInformation
}

func newCOMChain() (*comChain, error) {
	var c comChain

	var enumPtr uintptr
	hr, _, _ := coCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enumPtr)),
	)
	if err := checkHR(hr, "CoCreateInstance(MMDeviceEnumerator)"); err != nil {
		return nil, err
	}
	c.enumerator = iMMDeviceEnumerator{comObject{enumPtr}}

	dev, err := c.enumerator.GetDefaultAudioEndpoint()
	if err != nil {
		c.close()
		return nil, err
	}
	c.device = dev

	obj, err := c.device.Activate(&iidIAudioSessionManager2)
	if err != nil {
		c.close()
		return nil, err
	}
	c.sessionMgr = iAudioSessionManager2{obj}

	return &c, nil
}

func (c *comChain) close() {
	c.clearCache()
	c.sessionMgr.Release()
	c.device.Release()
	c.enumerator.Release()
}

func (c *comChain) clearCache() {
	c.cachedMeter.Release()
	c.cachedMeter = iAudioMeterInformation{}
	c.cachedPID = 0
}

// peakForPID returns the peak audio level for the given PID.
// Uses a cached meter when available, re-enumerates only when needed.
// Returns -1 if no session is found.
func (c *comChain) peakForPID(pid uint32) (float32, error) {
	// Try cached meter first
	if c.cachedPID == pid && c.cachedMeter.ptr != 0 {
		peak, err := c.cachedMeter.GetPeakValue()
		if err == nil {
			return peak, nil
		}
		// Meter went stale, re-enumerate
		c.clearCache()
	}

	sessionEnum, err := c.sessionMgr.GetSessionEnumerator()
	if err != nil {
		return -1, err
	}
	defer sessionEnum.Release()

	for i := range sessionEnum.GetCount() {
		session, err := sessionEnum.GetSession(i)
		if err != nil {
			continue
		}

		ctrl2, err := session.QueryInterface(&iidIAudioSessionControl2)
		if err != nil {
			session.Release()
			continue
		}

		sessionPID := iAudioSessionControl2{ctrl2}.GetProcessId()
		ctrl2.Release()

		if sessionPID != pid {
			session.Release()
			continue
		}

		// Found the session for our PID, now get its meter
		meterObj, err := session.QueryInterface(&iidIAudioMeterInformation)
		session.Release()
		if err != nil {
			return -1, fmt.Errorf("QI IAudioMeterInformation: %w", err)
		}

		meter := iAudioMeterInformation{meterObj}
		c.cachedPID = pid
		c.cachedMeter = meter

		return meter.GetPeakValue()
	}

	return -1, nil
}

func pidFromWindowTitle(title string) (uint32, error) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0, err
	}

	hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return 0, fmt.Errorf("window %q not found", title)
	}

	var pid uint32
	getWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return 0, fmt.Errorf("could not get PID for window %q", title)
	}
	return pid, nil
}

// wasapiMonitor implements Monitor using Windows WASAPI per-session peak metering.
// The COM chain is initialized once and reused across polls for efficiency. All COM calls are serialized on a dedicated OS thread.
type wasapiMonitor struct {
	windowTitle      string
	pollInterval     time.Duration
	gracePeriod      time.Duration
	silenceThreshold float32
	silenceDuration  time.Duration

	// Long-lived COM objects must be released on the same thread they were created on, so we run a COM chain on a dedicated OS thread and send it requests via a channel.
	chain     *comChain
	comThread chan comRequest
	ready     chan error
}

type comRequest struct {
	ctx    context.Context
	pid    uint32
	result chan comResult
}

type comResult struct {
	peak float32
	err  error
}

// NewMonitor creates a WASAPI-based audio monitor.
func NewMonitor(windowTitle string, gracePeriod time.Duration) (Monitor, error) {
	m := &wasapiMonitor{
		windowTitle:      windowTitle,
		pollInterval:     50 * time.Millisecond,
		gracePeriod:      gracePeriod,
		silenceThreshold: 0.001,
		silenceDuration:  150 * time.Millisecond,
		comThread:        make(chan comRequest),
		ready:            make(chan error, 1),
	}

	go m.comLoop()

	if err := <-m.ready; err != nil {
		return nil, err
	}

	return m, nil
}

// comLoop runs on a locked OS thread and owns all COM state.
func (m *wasapiMonitor) comLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := coInitializeEx.Call(0, coinitApartmentThreaded)
	if hr != 0 && hr != 1 {
		m.ready <- fmt.Errorf("CoInitializeEx: HRESULT %#x", hr)
		return
	}
	// Only uninitialize if we initialized (hr == 0). S_FALSE (1) means
	// someone else initialized on this thread, so we shouldn't tear it down.
	if hr == 0 {
		defer coUninitializeFn.Call()
	}

	chain, err := newCOMChain()
	if err != nil {
		m.ready <- err
		return
	}
	defer chain.close()
	m.chain = chain

	m.ready <- nil

	for req := range m.comThread {
		peak, err := chain.peakForPID(req.pid)
		req.result <- comResult{peak, err}
	}
}

func (m *wasapiMonitor) getPeak(ctx context.Context, pid uint32) (float32, error) {
	ch := make(chan comResult, 1)
	select {
	case m.comThread <- comRequest{ctx: ctx, pid: pid, result: ch}:
	case <-ctx.Done():
		return -1, ctx.Err()
	}
	select {
	case r := <-ch:
		return r.peak, r.err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (m *wasapiMonitor) WaitForSilence(ctx context.Context) error {
	pid, err := pidFromWindowTitle(m.windowTitle)
	if err != nil {
		return err
	}

	requiredCount := max(1, int(m.silenceDuration/m.pollInterval))
	graceDeadline := time.Now().Add(m.gracePeriod)
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	audioSeen := false
	silentCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			peak, err := m.getPeak(ctx, pid)
			if err != nil {
				return err
			}

			graceExpired := time.Now().After(graceDeadline)

			if peak < 0 {
				if graceExpired && !audioSeen {
					return ErrNoAudioSession
				}
				continue
			}

			if peak > m.silenceThreshold {
				audioSeen = true
				silentCount = 0
			} else if audioSeen {
				silentCount++
				if silentCount >= requiredCount {
					return nil
				}
			} else if graceExpired {
				return ErrNoAudioSession
			}
		}
	}
}

func (m *wasapiMonitor) PeakLevel() (float32, error) {
	pid, err := pidFromWindowTitle(m.windowTitle)
	if err != nil {
		return -1, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return m.getPeak(ctx, pid)
}

func (m *wasapiMonitor) Close() error {
	close(m.comThread)
	return nil
}
