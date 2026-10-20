package tui

import (
	"fmt"
	"strings"

	"github.com/juanrubio/outfitter/internal/domain"
)

func (m *Model) renderProfiles() []string {
	lines := m.headerLines("Profiles · Select an outfit profile")
	bodyHeight := m.height - len(lines) - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	boxWidth := minInt(m.width-4, 86)
	if boxWidth < 10 {
		boxWidth = maxInt(m.width, 10)
	}
	boxHeight := minInt(bodyHeight, 24)
	if boxHeight < 3 {
		boxHeight = bodyHeight
	}
	box := m.profileBox(boxWidth, boxHeight)
	lines = append(lines, m.centerBlock(box, boxWidth, bodyHeight)...)
	lines = append(lines, m.footerLines("↑↓/jk Select   Enter Apply profile to plan   Esc Cancel", "")...)
	return lines
}

func (m *Model) profileBox(width, height int) []string {
	if height < 2 {
		return []string{m.boxTop("SELECT OUTFITTER PROFILE (p)", width, "accent")}
	}
	innerWidth := width - 2
	innerHeight := height - 2

	content := make([][]span, 0, innerHeight)
	content = append(content, []span{{text: "Choose a preset bundle to queue missing tools for this machine:", token: "muted"}})

	for index, profile := range m.catalog.Profiles {
		if len(content)+3 > innerHeight {
			break
		}
		cursor := "  "
		nameToken := "text"
		if index == m.profileCursor {
			cursor = "> "
			nameToken = "accent"
		}
		badge := ""
		if profile.ID == m.activeProfileID {
			badge = " [active]"
		}
		right := fmt.Sprintf("%d tools%s", len(profile.Tools), badge)
		rightToken := "muted"
		if profile.ID == m.activeProfileID {
			rightToken = "good"
		}
		content = append(content, composeSpans(
			[]span{
				{text: cursor, token: "accent", bold: true},
				{text: fmt.Sprintf("%d. %s", index+1, profile.Name), token: nameToken, bold: true},
			},
			[]span{{text: right, token: rightToken}},
			innerWidth,
		))
		descriptionWidth := innerWidth - 8
		if descriptionWidth < 1 {
			descriptionWidth = 1
		}
		content = append(content, []span{{text: "    " + truncateDisplay(profile.Description, descriptionWidth), token: "muted"}})
	}

	for len(content) < innerHeight-1 {
		content = append(content, nil)
	}
	if len(content) >= innerHeight {
		content = content[:innerHeight-1]
	}
	content = append(content, []span{{text: "↑↓/jk Select   Enter Apply profile to plan   Esc Cancel", token: "accent", bold: true}})

	lines := []string{m.boxTop("SELECT OUTFITTER PROFILE (p)", width, "accent")}
	for _, spans := range content {
		lines = append(lines, m.boxContentLine(spans, width))
	}
	lines = append(lines, m.boxBottom(width))
	return lines
}

func (m *Model) centerBlock(box []string, boxWidth, height int) []string {
	if height < 1 {
		return nil
	}
	if boxWidth > m.width {
		boxWidth = m.width
	}
	left := maxInt((m.width-boxWidth)/2, 0)
	top := maxInt((height-len(box))/2, 0)
	lines := make([]string, 0, height)
	for index := 0; index < height; index++ {
		boxIndex := index - top
		if boxIndex < 0 || boxIndex >= len(box) {
			lines = append(lines, m.blankCell(m.width))
			continue
		}
		line := box[boxIndex]
		lineWidth := displayWidth(line)
		if lineWidth > m.width-left {
			line = clipStyledLine(line, m.width-left)
			lineWidth = displayWidth(line)
		}
		right := m.width - left - lineWidth
		if right < 0 {
			right = 0
		}
		lines = append(lines, m.blankCell(left)+line+m.blankCell(right))
	}
	return lines
}

func (m *Model) renderPlanScreen() []string {
	lines := m.headerLines("Plan & Apply · Dry-Run Review")
	bodyHeight := m.height - len(lines) - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	boxWidth := m.width - 4
	if boxWidth < 2 {
		boxWidth = maxInt(m.width, 2)
	}
	box := m.planReviewBox(boxWidth, bodyHeight)
	for _, line := range box {
		lines = append(lines, m.screenLineText(line, m.width))
	}
	lines = append(lines, m.footerLines("Enter Execute plan   u Queue all updates   Esc Return to catalog", "")...)
	return lines
}

func (m *Model) planReviewBox(width, height int) []string {
	title := "DRY-RUN EXECUTION PLAN (Nothing has been modified on disk yet)"
	titleToken := "accent"
	lines := []string{m.boxTop(title, width, titleToken)}
	if height < 2 {
		return lines
	}
	innerWidth := width - 2
	innerHeight := height - 2

	content := make([][]span, 0, innerHeight)
	if m.plan.IsEmpty() {
		content = append(content,
			nil,
			[]span{{text: "No pending changes in your plan.", token: "text", bold: true}},
			nil,
			[]span{{text: "Press Space on any tool in the catalog, or press [u] to queue all updates.", token: "muted"}},
		)
	} else {
		installs, updates, removals := m.plan.Counts()
		sudoNote := "[user-space only, no sudo]"
		if m.plan.NeedsSudo() {
			sudoNote = "[sudo required for apt]"
		}
		content = append(content,
			[]span{{text: fmt.Sprintf("Summary: +%d install · ↑%d update · -%d remove · %s", installs, updates, removals, sudoNote), token: "text", bold: true}},
			nil,
		)

		if m.plan.BootstrapStep != nil {
			content = append(content,
				[]span{{text: "0. BOOTSTRAP PREREQUISITE (mise)", token: "low", bold: true}},
				[]span{{text: "$ " + truncateDisplay(m.plan.BootstrapStep.Command, innerWidth-6), token: "accent"}},
				nil,
			)
		}

		content = m.appendPlanSection(content, "INSTALLATIONS", installs, m.plan.Installations, "+", "accent", innerWidth)
		content = m.appendPlanSection(content, "UPDATES", updates, m.plan.Updates, "↑", "low", innerWidth)
		content = m.appendPlanSection(content, "REMOVALS", removals, m.plan.Removals, "-", "error", innerWidth)
		content = appendStraySection(content, m.plan, innerWidth)
	}

	if len(content) > innerHeight {
		content = content[:innerHeight]
	}
	for len(content) < innerHeight {
		content = append(content, nil)
	}
	for _, spans := range content {
		lines = append(lines, m.boxContentLine(spans, width))
	}
	lines = append(lines, m.boxBottom(width))
	return lines
}

func (m *Model) appendPlanSection(
	content [][]span,
	title string,
	count int,
	steps []domain.PlanStep,
	symbol, token string,
	innerWidth int,
) [][]span {
	if count == 0 {
		return content
	}
	content = append(content, []span{{text: fmt.Sprintf("%s (%s%d)", title, symbol, count), token: token, bold: true}})
	for _, step := range steps {
		version := planStepVersion(step)
		backend := fmt.Sprintf("[%s", step.Tool.Backend)
		if step.Sudo {
			backend += "·sudo"
		}
		backend += "]"
		row := fmt.Sprintf("%s %s %s %s", symbol, padDisplay(step.Tool.Name, 20), padDisplay(version, 22), backend)
		command := "  $ " + step.Command
		row += truncateDisplay(command, maxInt(innerWidth-displayWidth(row)-2, 0))
		content = append(content, []span{{text: truncateDisplay(row, innerWidth), token: token}})
	}
	content = append(content, nil)
	return content
}

// appendStraySection warns about unmanaged binaries that stay on disk
// after a migrating update, with the exact command to remove each one.
func appendStraySection(content [][]span, plan domain.ActionPlan, innerWidth int) [][]span {
	var strays []domain.PlanStep
	for _, step := range plan.Steps() {
		if step.StrayBinaryPath != "" {
			strays = append(strays, step)
		}
	}
	if len(strays) == 0 {
		return content
	}
	content = append(content,
		[]span{{text: fmt.Sprintf("STRAY BINARIES (⚠%d)", len(strays)), token: "low", bold: true}},
	)
	for _, step := range strays {
		warning := fmt.Sprintf("⚠ %s: %s is not managed by %s and may shadow the update",
			step.Tool.Name, step.StrayBinaryPath, step.Tool.Backend)
		content = append(content,
			[]span{{text: truncateDisplay(warning, innerWidth), token: "low"}},
			[]span{{text: truncateDisplay("  remove after applying: $ "+step.StrayRemovalCommand(), innerWidth), token: "text"}},
		)
	}
	content = append(content, nil)
	return content
}

func planStepVersion(step domain.PlanStep) string {
	switch step.Action {
	case domain.ActionInstall:
		return "— → " + step.ToVersion
	case domain.ActionUpdate:
		return step.FromVersion + " → " + step.ToVersion
	case domain.ActionRemove:
		return step.FromVersion
	default:
		return step.ToVersion
	}
}

func (m *Model) renderApplying() []string {
	done, total, failed := m.executionCounts()
	lines := m.headerLines(fmt.Sprintf("Executing Action Plan · %d of %d completed", done, total))
	bodyHeight := m.height - len(lines) - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	boxWidth := m.width - 4
	if boxWidth < 2 {
		boxWidth = maxInt(m.width, 2)
	}
	box := m.applyBox(boxWidth, bodyHeight, done, total, failed)
	for _, line := range box {
		lines = append(lines, m.screenLineText(line, m.width))
	}
	lines = append(lines, m.footerLines("Esc Return to catalog (background task continues safely)   l Full log file", "")...)
	return lines
}

func (m *Model) executionCounts() (done, total, failed int) {
	total = len(m.execSteps)
	for _, step := range m.execSteps {
		switch step.status {
		case StepDone:
			done++
		case StepFailed:
			failed++
		}
	}
	return done, total, failed
}

func (m *Model) applyBox(width, height int, done, total, failed int) []string {
	lines := []string{m.boxTop("LIVE EXECUTION PROGRESS", width, "accent")}
	if height < 2 {
		return lines
	}
	innerWidth := width - 2
	innerHeight := height - 2
	content := make([][]span, 0, innerHeight)

	percent := 0
	if total > 0 {
		percent = done * 100 / total
	}
	barWidth := clampInt(innerWidth-38, 1, 40)
	filled := 0
	if total > 0 {
		filled = barWidth * done / total
	}
	progress := []span{
		{text: "Overall progress  ", token: "muted"},
		{text: strings.Repeat("█", filled), token: "good"},
		{text: strings.Repeat("░", barWidth-filled), token: "border"},
		{text: fmt.Sprintf(" %3d%% (%d/%d steps · %d failed)", percent, done, total, failed), token: "accent", bold: true},
	}
	content = append(content, progress, nil)

	logReserve := clampInt(innerHeight/3, 3, 7)
	stepLimit := innerHeight - 2 - logReserve
	if stepLimit < 0 {
		stepLimit = 0
	}
	for index, step := range m.execSteps {
		if index >= stepLimit {
			break
		}
		content = append(content, m.executionStepLine(step, innerWidth))
	}
	if len(m.execSteps) > stepLimit && stepLimit > 0 {
		content = content[:len(content)-1]
		content = append(content, []span{{text: fmt.Sprintf("… %d more steps", len(m.execSteps)-stepLimit+1), token: "muted"}})
	}

	for len(content) < innerHeight-logReserve {
		content = append(content, nil)
	}

	content = append(content, []span{{text: strings.Repeat("─", innerWidth), token: "border"}})
	active := m.activeExecutionStep()
	activeName := "plan"
	activeCommand := ""
	if active != nil {
		activeName = active.step.Tool.Name
		activeCommand = active.step.Command
	}
	content = append(content, []span{{text: "LIVE OUTPUT · " + activeName, token: "accent", bold: true}})
	if activeCommand != "" {
		content = append(content, []span{{text: "$ " + truncateDisplay(activeCommand, innerWidth-3), token: "text"}})
	}
	if active != nil {
		for index, line := range active.logs {
			if index == 0 {
				continue
			}
			if len(content) >= innerHeight {
				break
			}
			content = append(content, []span{{text: truncateDisplay(line, innerWidth), token: "muted"}})
		}
	}

	for len(content) < innerHeight {
		content = append(content, nil)
	}
	if len(content) > innerHeight {
		content = content[:innerHeight]
	}
	for _, spans := range content {
		lines = append(lines, m.boxContentLine(spans, width))
	}
	lines = append(lines, m.boxBottom(width))
	return lines
}

func (m *Model) executionStepLine(step executionStep, innerWidth int) []span {
	icon := "·"
	token := "muted"
	bold := false
	switch step.status {
	case StepDone:
		icon = "✓"
		token = "good"
	case StepRunning:
		icon = "⠋"
		token = "accent"
		bold = true
	case StepFailed:
		icon = "✗"
		token = "error"
	}
	version := planStepVersion(step.step)
	backend := fmt.Sprintf("[%s]", step.step.Tool.Backend)
	left := []span{
		{text: icon + " ", token: token, bold: bold},
		{text: padDisplay(step.step.Tool.Name, 20), token: token, bold: bold},
		{text: padDisplay(version, 26), token: token},
		{text: backend, token: "muted"},
	}
	right := []span{{text: step.elapsed, token: token}}
	return composeSpans(left, right, innerWidth)
}

func (m *Model) activeExecutionStep() *executionStep {
	for index := range m.execSteps {
		if m.execSteps[index].status == StepRunning {
			return &m.execSteps[index]
		}
	}
	for index := len(m.execSteps) - 1; index >= 0; index-- {
		if m.execSteps[index].status == StepDone || m.execSteps[index].status == StepFailed {
			return &m.execSteps[index]
		}
	}
	if len(m.execSteps) > 0 {
		return &m.execSteps[0]
	}
	return nil
}

func (m *Model) renderHelp() []string {
	lines := m.headerLines("Keyboard Reference")
	bodyHeight := m.height - len(lines) - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	boxWidth := m.width - 4
	if boxWidth < 2 {
		boxWidth = maxInt(m.width, 2)
	}
	box := m.helpBox(boxWidth, bodyHeight)
	for _, line := range box {
		lines = append(lines, m.screenLineText(line, m.width))
	}
	lines = append(lines, m.footerLines("Esc or ? Close help", "")...)
	return lines
}

func (m *Model) helpBox(width, height int) []string {
	lines := []string{m.boxTop("OUTFITTER KEYBOARD SHORTCUTS (?)", width, "accent")}
	if height < 2 {
		return lines
	}
	innerWidth := width - 2
	innerHeight := height - 2
	pairs := [][2]string{
		{"↑ / ↓ or j / k", "Move selection between tools in the catalog"},
		{"PgUp / PgDn", "Page selection by ten tools"},
		{"1 / 2 / 3 / 4", "Jump to Category 1 (Harnesses), 2 (AI TUIs), 3 (Terminal), 4 (Utilities)"},
		{"Tab", "Collapse or expand the currently focused category"},
		{"Space", "Toggle desired action for selected tool ([ ] ↔ [+], [!] → [↑] → [-], [✓] ↔ [-])"},
		{"u", "Queue updates ([↑]) for ALL outdated tools in the catalog at once"},
		{"i / + / U", "Queue install ([+]) or update ([↑]) for the focused tool"},
		{"g", "Toggle all tools in the focused category"},
		{"p", "Open Preset Profile selector (Full Desktop, Headless Server, Harnesses, Minimal)"},
		{"a", "Open Plan & Apply dry-run confirmation screen"},
		{"x", "Reset/clear all pending actions in the current plan"},
		{"f", "Cycle filter: All → Outdated → Pending → Installed → Not Installed"},
		{"/", "Incremental search by tool name, binary, backend, package, or description"},
		{"b / B", "Bootstrap state demo / fix mise shell integration"},
		{"r", "Refresh installed binary checks and upstream version readings"},
		{"Esc / q", "Close active overlay or return to the catalog desk"},
	}

	content := make([][]span, 0, innerHeight)
	for _, pair := range pairs {
		if len(content) >= innerHeight {
			break
		}
		keyWidth := 18
		descriptionWidth := innerWidth - keyWidth - 1
		if descriptionWidth < 1 {
			descriptionWidth = 1
		}
		content = append(content, []span{
			{text: padDisplay(pair[0], keyWidth), token: "accent", bold: true},
			{text: " " + truncateDisplay(pair[1], descriptionWidth), token: "text"},
		})
	}
	for len(content) < innerHeight {
		content = append(content, nil)
	}
	for _, spans := range content {
		lines = append(lines, m.boxContentLine(spans, width))
	}
	lines = append(lines, m.boxBottom(width))
	return lines
}
