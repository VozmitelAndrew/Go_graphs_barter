package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"runtime"
	"slices"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"
	_ "github.com/AllenDang/cimgui-go/impl/glfw"
	_ "github.com/AllenDang/cimgui-go/impl/opengl3"

	"gographsbarter/matching"
)

const (
	nodeRadius  = 22
	arrowLength = 12
	arrowWidth  = 6
	panelWidth  = 300

	spawnAttempts = 20
	spawnGap      = 8
)

var (
	colEdge      = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(0.6, 0.6, 0.6, 1))
	colCycleEdge = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1.0, 0.45, 0.2, 1))
	colPreview   = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1.0, 0.85, 0.3, 0.6))
	colNode      = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(0.25, 0.55, 0.95, 1))
	colNodeHover = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(0.45, 0.7, 1.0, 1))
	colSelected  = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1.0, 0.85, 0.3, 1))
	colText      = imgui.ColorConvertFloat4ToU32(imgui.NewVec4(1, 1, 1, 1))

	colActiveButton = imgui.NewVec4(0.8, 0.45, 0.15, 1)
)

type editMode uint8

const (
	modeIdle editMode = iota
	modeDeleteVertex
	modeAddEdge
	modeDeleteEdge
)

func (m editMode) hint() string {
	switch m {
	case modeDeleteVertex:
		return "Click a vertex to delete it."
	case modeAddEdge:
		return "Click the source vertex, then the target."
	case modeDeleteEdge:
		return "Click the source vertex, then the target of the edge to delete."
	}
	return "Drag vertices with the mouse."
}

type graphView struct {
	adj     [][]int
	cycle   []int
	inCycle map[[2]int]bool

	pos        []imgui.Vec2
	placed     bool
	canvasSize imgui.Vec2

	hover   int
	drag    int
	grabOff imgui.Vec2

	mode    editMode
	pending int
}

func newGraphView(adj [][]int) *graphView {
	g := &graphView{
		adj:     adj,
		hover:   -1,
		drag:    -1,
		pending: -1,
	}
	g.recompute()
	return g
}

func init() {
	runtime.LockOSThread()
}

func main() {
	adj := [][]int{
		{1},
		{2},
		{0, 3},
		{},
	}
	view := newGraphView(adj)

	be, err := backend.CreateBackend(glfwbackend.NewGLFWBackend())
	if err != nil {
		panic(err)
	}
	be.SetBgColor(imgui.NewVec4(0.1, 0.1, 0.12, 1))
	be.CreateWindow("barter graph", 1200, 750)
	be.Run(view.frame)
}

func (g *graphView) frame() {
	vp := imgui.MainViewport()
	work, size := vp.WorkPos(), vp.WorkSize()

	imgui.SetNextWindowPos(imgui.NewVec2(work.X+panelWidth, work.Y))
	imgui.SetNextWindowSize(imgui.NewVec2(size.X-panelWidth, size.Y))
	canvasFlags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsNoMove |
		imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoBringToFrontOnFocus |
		imgui.WindowFlagsNoBackground
	if imgui.BeginV("canvas", nil, canvasFlags) {
		origin := imgui.CursorScreenPos()
		g.canvasSize = imgui.ContentRegionAvail()
		if !g.placed {
			g.pos = circleLayout(len(g.adj), g.canvasSize)
			g.placed = true
		}
		g.handleMouse(origin, imgui.IsWindowHovered())
		g.draw(imgui.WindowDrawList(), origin)
	}
	imgui.End()

	imgui.SetNextWindowPos(work)
	imgui.SetNextWindowSize(imgui.NewVec2(panelWidth, size.Y))
	panelFlags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsNoMove |
		imgui.WindowFlagsNoSavedSettings
	if imgui.BeginV("panel", nil, panelFlags) {
		g.drawPanel()
	}
	imgui.End()

	g.handleKeys()
}

func (g *graphView) handleKeys() {
	if imgui.CurrentIO().WantTextInput() {
		return
	}
	if imgui.IsKeyPressedBool(imgui.KeyEscape) {
		g.setMode(modeIdle)
	}
	if imgui.IsKeyPressedBoolV(imgui.KeyA, false) {
		g.toggleMode(modeAddEdge)
	}
	if imgui.IsKeyPressedBoolV(imgui.KeyD, false) {
		g.toggleMode(modeDeleteEdge)
	}
	if imgui.IsKeyPressedBoolV(imgui.KeyC, false) {
		g.setMode(modeIdle)
		g.clear()
	}
}

func (g *graphView) drawPanel() {
	imgui.SeparatorText("Graph")
	imgui.Text(fmt.Sprintf("vertices: %d", len(g.adj)))
	imgui.Text(fmt.Sprintf("cycle: %v", g.cycle))

	imgui.SeparatorText("Edit")
	half := (imgui.ContentRegionAvail().X - imgui.CurrentStyle().ItemSpacing().X) / 2

	if imgui.ButtonV("Add vertex", imgui.NewVec2(half, 0)) {
		g.setMode(modeIdle)
		g.addVertex()
	}
	imgui.SameLine()
	g.modeButton("Delete vertex", modeDeleteVertex, half)

	g.modeButton("Add edge (A)", modeAddEdge, half)
	imgui.SameLine()
	g.modeButton("Delete edge (D)", modeDeleteEdge, half)

	if imgui.ButtonV("Clear (C)", imgui.NewVec2(imgui.ContentRegionAvail().X, 0)) {
		g.setMode(modeIdle)
		g.clear()
	}

	imgui.Spacing()
	imgui.TextWrapped(g.mode.hint())
	if g.mode != modeIdle {
		if imgui.Button("Cancel (Esc)") {
			g.setMode(modeIdle)
		}
	}
}

func (g *graphView) modeButton(label string, m editMode, width float32) {
	active := g.mode == m
	if active {
		imgui.PushStyleColorVec4(imgui.ColButton, colActiveButton)
	}
	if imgui.ButtonV(label, imgui.NewVec2(width, 0)) {
		g.toggleMode(m)
	}
	if active {
		imgui.PopStyleColor()
	}
}

func (g *graphView) toggleMode(m editMode) {
	if g.mode == m {
		g.setMode(modeIdle)
	} else {
		g.setMode(m)
	}
}

func (g *graphView) setMode(m editMode) {
	g.mode = m
	g.pending = -1
	g.drag = -1
}

func (g *graphView) handleMouse(origin imgui.Vec2, canvasHovered bool) {
	mouse := imgui.MousePos().Sub(origin)

	g.hover = -1
	if canvasHovered || g.drag >= 0 {
		g.hover = g.nodeAt(mouse)
	}

	clicked := canvasHovered && g.hover >= 0 && imgui.IsMouseClickedBool(imgui.MouseButtonLeft)
	if clicked {
		g.onVertexClick(g.hover, mouse)
	}

	if g.drag >= 0 {
		if imgui.IsMouseDown(imgui.MouseButtonLeft) {
			g.pos[g.drag] = mouse.Add(g.grabOff)
		} else {
			g.drag = -1
		}
	}
}

func (g *graphView) onVertexClick(v int, mouse imgui.Vec2) {
	switch g.mode {
	case modeIdle:
		g.drag = v
		g.grabOff = g.pos[v].Sub(mouse)

	case modeDeleteVertex:
		g.deleteVertex(v)
		g.setMode(modeIdle)

	case modeAddEdge, modeDeleteEdge:
		if g.pending < 0 {
			g.pending = v
			return
		}
		if v == g.pending {
			return
		}
		if g.mode == modeAddEdge {
			g.addEdge(g.pending, v)
		} else {
			g.deleteEdge(g.pending, v)
		}
		g.setMode(modeIdle)
	}
}

func (g *graphView) addVertex() {
	g.adj = append(g.adj, nil)
	g.pos = append(g.pos, g.spawnPoint())
	g.recompute()
}

func (g *graphView) spawnPoint() imgui.Vec2 {
	const margin = nodeRadius + spawnGap
	random := func() imgui.Vec2 {
		return imgui.NewVec2(
			margin+rand.Float32()*max(g.canvasSize.X-2*margin, 0),
			margin+rand.Float32()*max(g.canvasSize.Y-2*margin, 0),
		)
	}

	for range spawnAttempts {
		if p := random(); !g.collides(p) {
			return p
		}
	}
	return random()
}

func (g *graphView) collides(p imgui.Vec2) bool {
	const minDist = 2*nodeRadius + spawnGap
	for _, q := range g.pos {
		d := p.Sub(q)
		if d.X*d.X+d.Y*d.Y < minDist*minDist {
			return true
		}
	}
	return false
}

func (g *graphView) deleteVertex(v int) {
	g.adj = slices.Delete(g.adj, v, v+1)
	g.pos = slices.Delete(g.pos, v, v+1)

	for u, nbrs := range g.adj {
		out := nbrs[:0]
		for _, w := range nbrs {
			switch {
			case w == v:
			case w > v:
				out = append(out, w-1)
			default:
				out = append(out, w)
			}
		}
		g.adj[u] = out
	}
	g.recompute()
}

func (g *graphView) clear() {
	g.adj = nil
	g.pos = nil
	g.hover = -1
	g.recompute()
}

func (g *graphView) addEdge(u, v int) {
	if !slices.Contains(g.adj[u], v) {
		g.adj[u] = append(g.adj[u], v)
		g.recompute()
	}
}

func (g *graphView) deleteEdge(u, v int) {
	if i := slices.Index(g.adj[u], v); i >= 0 {
		g.adj[u] = slices.Delete(g.adj[u], i, i+1)
		g.recompute()
	}
}

func (g *graphView) recompute() {
	g.cycle = matching.FindCycle(g.adj)
	g.inCycle = cycleEdges(g.cycle)
}

func (g *graphView) nodeAt(p imgui.Vec2) int {
	for i := len(g.pos) - 1; i >= 0; i-- {
		d := p.Sub(g.pos[i])
		if d.X*d.X+d.Y*d.Y <= nodeRadius*nodeRadius {
			return i
		}
	}
	return -1
}

func (g *graphView) draw(dl *imgui.DrawList, origin imgui.Vec2) {
	screen := func(i int) imgui.Vec2 { return origin.Add(g.pos[i]) }

	for u, nbrs := range g.adj {
		for _, v := range nbrs {
			col := colEdge
			if g.inCycle[[2]int{u, v}] {
				col = colCycleEdge
			}
			drawArrow(dl, screen(u), screen(v), col)
		}
	}

	if g.pending >= 0 {
		dl.AddLineArgs(screen(g.pending), imgui.MousePos(), colPreview, 2)
	}

	for i := range g.pos {
		col := colNode
		if i == g.hover || i == g.drag {
			col = colNodeHover
		}
		p := screen(i)
		dl.AddCircleFilled(p, nodeRadius, col)
		if i == g.pending {
			dl.AddCircleV(p, nodeRadius+4, colSelected, 0, 3)
		}

		label := fmt.Sprint(i)
		dl.AddTextVec2(p.Sub(imgui.CalcTextSize(label).Div(2)), colText, label)
	}
}

func circleLayout(n int, size imgui.Vec2) []imgui.Vec2 {
	center := size.Div(2)
	r := min(size.X, size.Y)/2 - nodeRadius*2
	pos := make([]imgui.Vec2, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		pos[i] = center.Add(imgui.NewVec2(
			float32(math.Cos(a))*r,
			float32(math.Sin(a))*r,
		))
	}
	return pos
}

func cycleEdges(c []int) map[[2]int]bool {
	m := make(map[[2]int]bool, len(c))
	for i, u := range c {
		v := c[(i+1)%len(c)]
		m[[2]int{u, v}] = true
	}
	return m
}

func drawArrow(dl *imgui.DrawList, from, to imgui.Vec2, col uint32) {
	d := to.Sub(from)
	length := float32(math.Hypot(float64(d.X), float64(d.Y)))
	if length < 2*nodeRadius {
		return
	}
	dir := d.Div(length)
	normal := imgui.NewVec2(-dir.Y, dir.X)

	start := from.Add(dir.Mul(nodeRadius))
	tip := to.Sub(dir.Mul(nodeRadius))
	base := tip.Sub(dir.Mul(arrowLength))

	dl.AddLineArgs(start, base, col, 2)
	dl.AddTriangleFilled(tip, base.Add(normal.Mul(arrowWidth)), base.Sub(normal.Mul(arrowWidth)), col)
}
