//go:build linux && (amd64 || arm64)

package linux

import (
	"math"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Some GTK 3 versions report mode / integer scale as monitor geometry even
// when xdg-output supplies a different logical size (KWin fractional scaling).
// Layer-shell margins use the compositor's logical coordinates. Query those
// on a private Wayland queue so no GTK callbacks run inside the roundtrips.
// This is linked and loaded only when a panel uses layer-shell.
type layerWLMessage struct {
	name, signature *byte
	types           *ptr
}

type layerWLInterface struct {
	name        *byte
	version     int32
	methodCount int32
	methods     *layerWLMessage
	eventCount  int32
	events      *layerWLMessage
}

var layerGeometry struct {
	once      sync.Once
	display   func(ptr) ptr
	output    func(ptr) ptr
	queue     func(ptr) ptr
	freeQueue func(ptr)
	setQueue  func(ptr, ptr)
	roundtrip func(ptr, ptr) int32
	construct func(ptr, uint32, *uint64, ptr, uint32) ptr
	listen    func(ptr, *ptr, ptr) int32
	marshal   func(ptr, uint32, *uint64)
	destroy   func(ptr)
	registry  ptr
	manager   layerWLInterface
	xdg       layerWLInterface
	methods   [2]layerWLMessage
	events    [5]layerWLMessage
	types     [2]ptr
	globals   [2]ptr
	listener  [5]ptr
	globalID  uint32
	// All access is on GTK's main thread; callbacks exist once per process.
	monitors map[ptr]gdkRectangle
	observed map[ptr]bool
}

func initLayerGeometry() {
	l := &layerGeometry
	l.once.Do(func() {
		lib, err := open("libwayland-client.so.0")
		if err != nil {
			return
		}
		if !bind(libGDK, &l.display, "gdk_wayland_display_get_wl_display") ||
			!bind(libGDK, &l.output, "gdk_wayland_monitor_get_wl_output") ||
			!bind(lib, &l.queue, "wl_display_create_queue") ||
			!bind(lib, &l.freeQueue, "wl_event_queue_destroy") ||
			!bind(lib, &l.setQueue, "wl_proxy_set_queue") ||
			!bind(lib, &l.roundtrip, "wl_display_roundtrip_queue") ||
			!bind(lib, &l.construct, "wl_proxy_marshal_array_constructor_versioned") ||
			!bind(lib, &l.listen, "wl_proxy_add_listener") ||
			!bind(lib, &l.marshal, "wl_proxy_marshal_array") ||
			!bind(lib, &l.destroy, "wl_proxy_destroy") {
			return
		}
		l.registry, _ = purego.Dlsym(lib, "wl_registry_interface")
		output, _ := purego.Dlsym(lib, "wl_output_interface")
		if l.registry == 0 || output == 0 {
			return
		}
		l.xdg = layerWLInterface{name: cs("zxdg_output_v1"), version: 2, methodCount: 1, methods: &l.methods[0], eventCount: 5, events: &l.events[0]}
		l.types = [2]ptr{uintptr(unsafe.Pointer(&l.xdg)), output}
		l.methods = [2]layerWLMessage{{name: cs("destroy"), signature: cs("")}, {name: cs("get_xdg_output"), signature: cs("no"), types: &l.types[0]}}
		l.manager = layerWLInterface{name: cs("zxdg_output_manager_v1"), version: 2, methodCount: 2, methods: &l.methods[0]}
		for i, event := range []struct{ name, signature string }{{"logical_position", "ii"}, {"logical_size", "ii"}, {"done", ""}, {"name", "2s"}, {"description", "2s"}} {
			l.events[i] = layerWLMessage{name: cs(event.name), signature: cs(event.signature)}
		}
		l.globals[0] = purego.NewCallback(func(data, registry ptr, name uint32, iface ptr, version uint32) {
			if goStr(iface) == "zxdg_output_manager_v1" && version >= 2 {
				l.globalID = name
			}
		})
		l.globals[1] = purego.NewCallback(func(data, registry ptr, name uint32) {})
		l.listener[0] = purego.NewCallback(func(monitor, output ptr, x, y int32) {
			r := l.monitors[monitor]
			r.X, r.Y = x, y
			l.monitors[monitor] = r
		})
		l.listener[1] = purego.NewCallback(func(monitor, output ptr, width, height int32) {
			r := l.monitors[monitor]
			r.Width, r.Height = width, height
			l.monitors[monitor] = r
		})
		l.listener[2] = purego.NewCallback(func(data, output ptr) {})
		l.listener[3] = purego.NewCallback(func(data, output, value ptr) {})
		l.listener[4] = l.listener[3]
		l.observed = make(map[ptr]bool)
		cbLayerGeometryChanged = purego.NewCallback(func(monitor, pspec, data ptr) {
			refreshLayerGeometry()
			theBackend.h.DisplaysChanged()
		})
		layerRefreshDisplays = refreshLayerGeometry
		layerDisplayGeometry = func(m ptr, fallback gdkRectangle) gdkRectangle {
			if r := l.monitors[m]; r.Width > 0 && r.Height > 0 {
				return r
			}
			return fallback
		}
	})
	if layerRefreshDisplays != nil {
		layerRefreshDisplays()
	}
}

func refreshLayerGeometry() {
	l := &layerGeometry
	d := l.display(gdkDisplayGetDefault())
	q := l.queue(d)
	if q == 0 {
		return
	}
	defer l.freeQueue(q)
	zero := uint64(0)
	registry := l.construct(d, 1, &zero, l.registry, 1) // wl_display.get_registry
	if registry == 0 {
		return
	}
	defer l.destroy(registry)
	l.setQueue(registry, q)
	l.globalID = 0
	l.listen(registry, &l.globals[0], 0)
	if l.roundtrip(d, q) < 0 || l.globalID == 0 {
		return
	}
	args := [4]uint64{uint64(l.globalID), uint64(uintptr(unsafe.Pointer(l.manager.name))), 2, 0}
	manager := l.construct(registry, 0, &args[0], uintptr(unsafe.Pointer(&l.manager)), 2)
	if manager == 0 {
		return
	}
	defer func() { l.marshal(manager, 0, nil); l.destroy(manager) }()
	l.setQueue(manager, q)
	l.monitors = make(map[ptr]gdkRectangle)
	observed := make(map[ptr]bool)
	var outputs []ptr
	defer func() {
		for _, output := range outputs {
			l.marshal(output, 0, nil)
			l.destroy(output)
		}
	}()
	display := gdkDisplayGetDefault()
	for i, n := int32(0), gdkDisplayGetNMonitors(display); i < n; i++ {
		m := gdkDisplayGetMonitor(display, i)
		args := [2]uint64{0, uint64(l.output(m))}
		output := l.construct(manager, 1, &args[0], uintptr(unsafe.Pointer(&l.xdg)), 2)
		if output == 0 {
			continue
		}
		outputs = append(outputs, output)
		l.setQueue(output, q)
		l.listen(output, &l.listener[0], m)
		// Geometry notifications cover resolution/scale changes on an existing
		// output; monitor-added/removed alone do not report those changes.
		if !l.observed[m] {
			connect(m, "notify::geometry", cbLayerGeometryChanged, 0)
		}
		observed[m] = true
	}
	l.observed = observed
	if l.roundtrip(d, q) < 0 {
		l.monitors = nil
	}
}

// Set by layer-shell only, keeping native Wayland probing out of ordinary apps.
var layerRefreshDisplays func()
var layerDisplayGeometry func(ptr, gdkRectangle) gdkRectangle
var cbLayerGeometryChanged ptr

func layerMonitorGeometry(m ptr) gdkRectangle {
	var r gdkRectangle
	gdkMonitorGetGeometry(m, &r)
	if layerDisplayGeometry != nil {
		r = layerDisplayGeometry(m, r)
	}
	return r
}

func layerLogicalWorkArea(geo, work, logical gdkRectangle) gdkRectangle {
	// Keep the work area's insets while replacing GTK's rounded-scale extent.
	return gdkRectangle{X: logical.X + work.X - geo.X, Y: logical.Y + work.Y - geo.Y,
		Width: max(1, logical.Width-(geo.Width-work.Width)), Height: max(1, logical.Height-(geo.Height-work.Height))}
}

func layerMonitorAtPoint(x, y int) ptr {
	d := gdkDisplayGetDefault()
	// GDK's lookup uses the same rounded geometry; select in layer-shell's
	// coordinate space instead, including outputs with a negative origin.
	var nearest ptr
	best := math.Inf(1)
	for i, n := int32(0), gdkDisplayGetNMonitors(d); i < n; i++ {
		m := gdkDisplayGetMonitor(d, i)
		r := layerMonitorGeometry(m)
		dx := float64(max(int(r.X)-x, 0, x-int(r.X+r.Width)+1))
		dy := float64(max(int(r.Y)-y, 0, y-int(r.Y+r.Height)+1))
		if distance := dx*dx + dy*dy; distance < best {
			nearest, best = m, distance
		}
	}
	return nearest
}
