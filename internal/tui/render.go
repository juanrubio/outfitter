package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/juanrubio/outfitter/internal/domain"
)

type span struct {
	text  string
	token string
	bold  bool
}

type catalogRow struct {
	category  domain.Category
	tool      *domain.Tool
	installed int
	total     int
	outdated  int
	collapsed bool
}

func (m *Model) renderCatalog() []string {
	lines := m.headerLines("")
	listY := len(lines)
	listHeight := m.height - listY - 2
	if listHeight < 1 {
		listHeight = 1
	}

	rows := m.catalogRows()
	selectedRow := 0
	for index, row := range rows {
		if row.tool != nil && row.tool.ID == m.selected {
			selectedRow = index
			break
		}
	}
	if selectedRow < m.scrollOffset {
		m.scrollOffset = selectedRow
	}
	if selectedRow >= m.scrollOffset+listHeight {
		m.scrollOffset = selectedRow - listHeight + 1
	}
	maxOffset := len(rows) - listHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	visible := rows
	if len(visible) > listHeight {
		end := m.scrollOffset + listHeight
		if end > len(visible) {
			end = len(visible)
		}
		visible = visible[m.scrollOffset:end]
	}

	wide := m.width >= 100 && m.height >= 24
	listWidth := m.width - 4
	if wide {
		listWidth = int(float64(m.width-6) * 0.56)
	}

	leftLines := make([]string, 0, listHeight)
	for _, row := range visible {
		if row.tool == nil {
			leftLines = append(leftLines, m.renderSpans(m.categoryLine(row, listWidth), listWidth, ""))
			continue
		}
		selected := row.tool.ID == m.selected
		background := ""
		if selected {
			background = "surface"
		}
		leftLines = append(leftLines, m.renderSpans(m.toolLine(*row.tool, listWidth, selected), listWidth, background))
	}
	for len(leftLines) < listHeight {
		leftLines = append(leftLines, strings.Repeat(" ", listWidth))
	}

	if wide {
		tool, ok := m.selectedTool()
		if ok {
			rightWidth := m.width - listWidth - 6
			rightLines := m.inspectorBlock(tool, rightWidth, listHeight)
			for index := 0; index < listHeight; index++ {
				lines = append(lines, "  "+leftLines[index]+"  "+rightLines[index]+"  ")
			}
		} else {
			for _, line := range leftLines {
				lines = append(lines, "  "+line+"  ")
			}
		}
	} else {
		for _, line := range leftLines {
			lines = append(lines, m.screenLineText(line, m.width))
		}
	}

	position := fmt.Sprintf("%d tools", len(m.catalog.Tools))
	if len(rows) > listHeight {
		position = fmt.Sprintf("%d/%d", selectedRow+1, len(rows))
	}
	lines = append(lines, m.footerLines(m.catalogFooterHint(), position)...)
	return lines
}

func (m *Model) renderDetail() []string {
	lines := m.headerLines("Tool Inspector · Esc returns to catalog")
	blockHeight := m.height - len(lines) - 2
	if blockHeight < 1 {
		blockHeight = 1
	}
	tool, ok := m.selectedTool()
	if ok {
		block := m.inspectorBlock(tool, m.width-4, blockHeight)
		for _, line := range block {
			lines = append(lines, m.screenLineText(line, m.width))
		}
	}
	lines = append(lines, m.footerLines("Space Toggle action  u Queue updates  a Plan & Apply  Esc Back to catalog", "")...)
	return lines
}

func (m *Model) screenLineText(content string, width int) string {
	if width <= 4 {
		return truncateDisplay(content, width)
	}
	return "  " + content + "  "
}

func (m *Model) catalogFooterHint() string {
	if m.width >= 100 {
		return "↑↓/jk Move  Space Toggle  u Queue updates  g Group  p Profile  f Filter  / Search  a Apply  ? Help  q Quit"
	}
	return "jk Move  Space Toggle  Enter Details  u Updates  p Profile  a Apply  ? Help"
}

func (m *Model) headerLines(barTitle string) []string {
	total, installed, outdated, installs, updates, removals := m.counts()
	pending := installs + updates + removals
	summary := fmt.Sprintf("%d/%d installed · %d outdated · Plan: +%d ↑%d -%d", installed, total, outdated, installs, updates, removals)
	if pending == 0 {
		summary = fmt.Sprintf("%d/%d installed · %d outdated · Plan: clean", installed, total, outdated)
	}

	profileName := "Full Desktop"
	if profile, ok := m.catalog.ProfileByID(m.activeProfileID); ok {
		profileName = profile.Name
	}

	headerLeft := []span{{text: "OUTFITTER", token: "accent", bold: true}}
	miseStatus := "mise " + m.miseVersion + " ready"
	if !m.miseInstalled {
		miseStatus = "mise missing (auto-bootstrap queued)"
	}
	if m.width >= 96 {
		token := "muted"
		if !m.miseInstalled {
			token = "low"
		}
		headerLeft = append(headerLeft, span{text: fmt.Sprintf("  Profile: %s · %s", profileName, miseStatus), token: token})
	} else {
		shortProfile := "Desktop"
		if profile, ok := m.catalog.ProfileByID(m.activeProfileID); ok && profile.Short != "" {
			shortProfile = profile.Short
		}
		shortMise := "mise ready"
		if !m.miseInstalled {
			shortMise = "mise missing"
		}
		headerLeft = append(headerLeft, span{text: fmt.Sprintf("  %s · %s", shortProfile, shortMise), token: "muted"})
	}

	summaryToken := "muted"
	if pending > 0 {
		summaryToken = "accent"
	}
	lines := []string{
		m.composeLine(headerLeft, []span{{text: summary, token: summaryToken, bold: pending > 0}}, m.width, ""),
	}

	var barLeft []span
	if barTitle != "" {
		barLeft = append(barLeft, span{text: barTitle, token: "accent", bold: true})
	} else {
		barLeft = append(barLeft,
			span{text: "Filter: ", token: "muted"},
			span{text: m.filterLabel(), token: "accent", bold: true},
			span{text: "  [p] Profile", token: "text"},
			span{text: fmt.Sprintf("  [u] Queue %d updates", outdated), token: tokenForCount(outdated, "low")},
			span{text: fmt.Sprintf("  [a] Apply plan (%d)", pending), token: tokenForCount(pending, "good")},
		)
	}

	barRight := []span{{text: m.platformNotice, token: "muted"}}
	if !m.miseInstalled {
		barRight = []span{{text: "mise missing · verified bootstrap on apply", token: "low", bold: true}}
	}
	if barTitle != "" || m.width < 110 {
		barRight = nil
	}
	lines = append(lines, m.composeLine(barLeft, barRight, m.width, ""))
	if m.refreshing {
		lines = append(lines, m.refreshLine())
	}
	if m.editing || m.query != "" {
		lines = append(lines, m.searchLine())
	}
	if m.notice != "" {
		token := "accent"
		if m.confirmQuit {
			token = "low"
		}
		lines = append(lines, m.renderSpans([]span{{text: truncateDisplay(m.notice, m.width), token: token, bold: true}}, m.width, ""))
	}
	lines = append(lines, m.renderSpans([]span{{text: strings.Repeat("─", m.width), token: "border"}}, m.width, ""))
	return lines
}

func (m *Model) refreshLine() string {
	total := m.refreshTotal
	if total <= 0 {
		total = len(m.catalog.Tools)
	}
	checked := m.refreshChecked
	if checked > total {
		checked = total
	}
	barWidth := 20
	filled := 0
	if total > 0 {
		filled = (checked * barWidth) / total
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
	phase := m.refreshPhase
	if phase == "" {
		phase = "Discovering tool states…"
	}
	left := []span{
		{text: "  " + m.spinnerFrame() + " ", token: "accent", bold: true},
		{text: phase + " ", token: "text", bold: true},
		{text: bar, token: "accent"},
		{text: fmt.Sprintf(" %d/%d checked", checked, total), token: "muted"},
	}
	return m.renderSpans(left, m.width, "")
}

func (m *Model) searchLine() string {
	spans := []span{
		{text: "/ ", token: "accent", bold: true},
		{text: m.query, token: "text"},
		{text: "▏", token: "accent", bold: true},
		{text: "  (Esc to clear)", token: "muted"},
	}
	return m.renderSpans(spans, m.width, "")
}

func tokenForCount(count int, token string) string {
	if count == 0 {
		return "muted"
	}
	return token
}

func (m *Model) footerLines(hint, position string) []string {
	lines := []string{m.renderSpans([]span{{text: strings.Repeat("─", m.width), token: "border"}}, m.width, "")}
	left := []span{{text: hint, token: "muted"}}
	var right []span
	if position != "" {
		right = []span{{text: position, token: "accent", bold: true}}
	}
	lines = append(lines, m.composeLine(left, right, m.width, ""))
	return lines
}

func (m *Model) catalogRows() []catalogRow {
	rows := make([]catalogRow, 0, len(m.catalog.Tools)+len(m.catalog.Categories))
	for _, category := range m.catalog.Categories {
		tools := m.catalog.ToolsByCategory(category.ID)
		if len(tools) == 0 {
			continue
		}
		installed := 0
		outdated := 0
		for _, tool := range tools {
			state := m.states[tool.ID]
			if state.IsInstalled() {
				installed++
			}
			if state.IsOutdated() {
				outdated++
			}
		}
		visibleTools := make([]domain.Tool, 0, len(tools))
		for _, tool := range tools {
			if m.toolMatches(tool) {
				visibleTools = append(visibleTools, tool)
			}
		}
		if len(visibleTools) == 0 {
			continue
		}
		rows = append(rows, catalogRow{
			category:  category,
			installed: installed,
			total:     len(tools),
			outdated:  outdated,
			collapsed: m.collapsed[category.ID],
		})
		if m.collapsed[category.ID] {
			continue
		}
		for index := range visibleTools {
			tool := visibleTools[index]
			rows = append(rows, catalogRow{tool: &tool})
		}
	}
	return rows
}

func (m *Model) categoryLine(row catalogRow, width int) []span {
	arrow := "▾"
	if row.collapsed {
		arrow = "▸"
	}
	upNote := ""
	if row.outdated > 0 {
		upNote = fmt.Sprintf(" · %d outdated", row.outdated)
	}
	title := fmt.Sprintf("%s %d · %s (%d/%d%s)", arrow, row.category.Num, row.category.Title, row.installed, row.total, upNote)
	return []span{{text: truncateDisplay(title, width), token: "text", bold: true}}
}

func (m *Model) toolLine(tool domain.Tool, width int, selected bool) []span {
	state := m.states[tool.ID]
	action := m.actions[tool.ID]
	marker, markerToken, _ := domain.Marker(action, state.IsInstalled(), state.IsOutdated())

	nameWidth := clampInt(width-34, 14, 24)
	versionWidth := 19
	fixedWidth := 1 + 1 + 4 + 1 + versionWidth
	backendWidth := 0
	if width >= 54 {
		remaining := width - fixedWidth - nameWidth
		if remaining > 0 {
			backendWidth = minInt(9, remaining)
		}
	}
	gapWidth := width - fixedWidth - nameWidth - backendWidth
	if gapWidth < 0 {
		gapWidth = 0
	}

	cursor := " "
	cursorToken := "muted"
	if selected {
		cursor = ">"
		cursorToken = "accent"
	}
	nameToken := "text"
	if selected {
		nameToken = "accent"
	} else if !state.IsInstalled() {
		nameToken = "muted"
	}

	versionText, versionToken := m.toolVersion(state, action)
	spans := []span{
		{text: cursor, token: cursorToken, bold: true},
		{text: " ", token: "text"},
		{text: padDisplay(marker, 4), token: markerToken, bold: true},
		{text: padDisplay(tool.Name, nameWidth), token: nameToken, bold: selected},
		{text: " ", token: "text"},
		{text: padDisplay(versionText, versionWidth), token: versionToken, bold: action != "" && action != domain.ActionNone},
		{text: strings.Repeat(" ", gapWidth), token: "text"},
	}
	if backendWidth > 0 {
		backendToken := "muted"
		if tool.Backend == domain.BackendAPT {
			backendToken = "low"
		}
		spans = append(spans, span{text: padDisplay(string(tool.Backend), backendWidth), token: backendToken})
	}
	return spans
}

func (m *Model) toolVersion(state domain.ToolState, action domain.DesiredAction) (string, string) {
	if action == domain.ActionInstall {
		if state.LatestVersion == "" && state.CheckStatus == domain.StatusChecking {
			return "— → " + m.spinnerFrame() + " checking…", "accent"
		}
		return "— → " + state.LatestVersion, "accent"
	}
	if action == domain.ActionUpdate || state.IsOutdated() {
		token := "low"
		if action == domain.ActionUpdate {
			token = "accent"
		}
		return shorten(state.InstalledVersion, 8) + " → " + shorten(state.LatestVersion, 8), token
	}
	if state.IsInstalled() {
		if state.CheckStatus == domain.StatusChecking {
			return shorten(state.InstalledVersion, 12) + " " + m.spinnerFrame(), "good"
		}
		return shorten(state.InstalledVersion, 15), "good"
	}
	if state.CheckStatus == domain.StatusChecking && state.LatestVersion == "" {
		return m.spinnerFrame() + " checking…", "muted"
	}
	return fmt.Sprintf("— (%s)", shorten(state.LatestVersion, 8)), "muted"
}

func (m *Model) inspectorBlock(tool domain.Tool, width, height int) []string {
	planHeight := 7
	if height >= 28 {
		planHeight = 9
	}
	detailHeight := height - planHeight - 1
	if detailHeight < 4 {
		detailHeight = height - 2
		planHeight = 1
	}
	lines := m.inspectorBox(tool, width, detailHeight)
	if planHeight > 1 && len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
		lines = append(lines, m.planBox(width, planHeight)...)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func (m *Model) inspectorBox(tool domain.Tool, width, height int) []string {
	if height < 2 {
		return []string{strings.Repeat(" ", width)}
	}
	state := m.states[tool.ID]
	action := m.actions[tool.ID]
	marker, markerToken, markerLabel := domain.Marker(action, state.IsInstalled(), state.IsOutdated())

	title := "INSPECTOR · " + tool.Binary
	top := m.boxTop(title, width, "accent")

	innerWidth := width - 2
	innerHeight := height - 2
	content := make([][]span, 0, innerHeight)

	backendText := string(tool.Backend)
	if tool.Sudo {
		backendText += " [sudo]"
	}
	backendToken := "accent"
	if tool.Sudo {
		backendToken = "low"
	}
	content = append(content, composeSpans(
		[]span{{text: tool.Name, token: "text", bold: true}},
		[]span{{text: backendText, token: backendToken, bold: true}},
		innerWidth,
	))

	description := wrapText(tool.Description, innerWidth)
	if len(description) > 2 {
		description = description[:2]
	}
	for _, line := range description {
		content = append(content, []span{{text: line, token: "muted"}})
	}
	content = append(content, nil)

	installedText := state.InstalledVersion
	installedToken := "good"
	if installedText == "" {
		if state.CheckStatus == domain.StatusChecking && m.refreshChecked == 0 {
			installedText = m.spinnerFrame() + " scanning local system…"
			installedToken = "accent"
		} else {
			installedText = "— (not installed)"
			installedToken = "muted"
		}
	}
	latestText := state.LatestVersion
	latestToken := "text"
	if latestText == "" && state.CheckStatus == domain.StatusChecking {
		latestText = m.spinnerFrame() + " checking upstream…"
		latestToken = "accent"
	} else if state.IsOutdated() {
		latestText += " (update available)"
		latestToken = "low"
	}
	binaryPath := state.BinaryPath
	if binaryPath == "" {
		binaryPath = "~/.local/bin/" + tool.Binary
	}
	provenance := state.ProvenanceDetail
	if provenance == "" {
		provenance = string(state.Provenance)
	}
	profiles := m.profileNames(tool)

	kv := []struct {
		label string
		value string
		token string
	}{
		{"State", marker + " " + markerLabel, markerToken},
		{"Installed", installedText, installedToken},
		{"Latest", latestText, latestToken},
		{"Binary path", binaryPath, "muted"},
		{"Package", tool.Package, "text"},
		{"Provenance", provenance, "muted"},
		{"Profiles", profiles, "text"},
	}
	for _, row := range kv {
		content = append(content, []span{
			{text: padDisplay(row.label, 12), token: "muted"},
			{text: " " + truncateDisplay(row.value, innerWidth-13), token: row.token, bold: row.label == "State"},
		})
	}

	commandLabel, commandText := toolCommand(tool, state, action)

	reserve := 0
	if innerHeight >= 6 {
		reserve = 3
	}
	maxContent := innerHeight - reserve
	if maxContent < 0 {
		maxContent = 0
	}
	if len(content) > maxContent {
		content = content[:maxContent]
	}
	for len(content) < maxContent {
		content = append(content, nil)
	}
	if reserve > 0 {
		content = append(content, nil,
			[]span{{text: strings.ToUpper(commandLabel), token: "muted", bold: true}},
			[]span{{text: "$ " + commandText, token: "accent"}},
		)
	}

	lines := []string{top}
	for _, spans := range content {
		lines = append(lines, m.boxContentLine(spans, width))
	}
	lines = append(lines, m.boxBottom(width))
	return lines
}

func (m *Model) planBox(width, height int) []string {
	installs, updates, removals := m.plan.Counts()
	pending := installs + updates + removals
	title := fmt.Sprintf("PENDING ACTION PLAN (%d)", pending)
	titleToken := "accent"
	if pending == 0 {
		titleToken = "border"
	}

	innerHeight := height - 2
	if innerHeight < 1 {
		return []string{m.boxTop(title, width, titleToken)}
	}

	content := make([][]span, 0, innerHeight)
	if !m.miseInstalled {
		content = append(content, []span{{text: "★ Bootstrap ~/.local/bin/mise v" + m.miseVersion, token: "low", bold: true}})
	}
	content = append(content,
		[]span{{text: fmt.Sprintf("+ Install (%d): %s", installs, m.queuedBinaries(domain.ActionInstall)), token: tokenForCount(installs, "accent")}},
		[]span{{text: fmt.Sprintf("↑ Update  (%d): %s", updates, m.queuedBinaries(domain.ActionUpdate)), token: tokenForCount(updates, "low")}},
		[]span{{text: fmt.Sprintf("- Remove  (%d): %s", removals, m.queuedBinaries(domain.ActionRemove)), token: tokenForCount(removals, "error")}},
	)

	for len(content) < innerHeight-1 {
		content = append(content, nil)
	}
	if innerHeight > 0 {
		if len(content) >= innerHeight {
			content = content[:innerHeight-1]
		}
		content = append(content, []span{{text: "Press [a] to review & apply · [x] reset plan", token: tokenForCount(pending, "good")}})
	}

	lines := []string{m.boxTop(title, width, titleToken)}
	for _, spans := range content {
		lines = append(lines, m.boxContentLine(spans, width))
	}
	lines = append(lines, m.boxBottom(width))
	return lines
}

func (m *Model) queuedBinaries(action domain.DesiredAction) string {
	names := make([]string, 0)
	for _, tool := range m.catalog.Tools {
		if m.actions[tool.ID] == action {
			names = append(names, tool.Binary)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func (m *Model) profileNames(tool domain.Tool) string {
	names := make([]string, 0, len(tool.Profiles))
	for _, id := range tool.Profiles {
		profile, ok := m.catalog.ProfileByID(id)
		if !ok {
			continue
		}
		if profile.Short != "" {
			names = append(names, profile.Short)
		} else {
			names = append(names, profile.Name)
		}
	}
	if len(names) == 0 {
		return "Custom"
	}
	return strings.Join(names, " · ")
}

func toolCommand(tool domain.Tool, state domain.ToolState, action domain.DesiredAction) (string, string) {
	switch action {
	case domain.ActionRemove:
		return "UNINSTALL CMD", tool.RemoveCommand()
	case domain.ActionInstall:
		return "INSTALL CMD", tool.InstallCommand()
	default:
		if state.IsInstalled() {
			return "UPDATE CMD", tool.UpdateCommand()
		}
		return "INSTALL CMD", tool.InstallCommand()
	}
}

func (m *Model) boxTop(title string, width int, titleToken string) string {
	if width < 2 {
		return strings.Repeat(" ", maxInt(width, 0))
	}
	maxTitle := width - 6
	if maxTitle < 0 {
		maxTitle = 0
	}
	title = truncateDisplay(title, maxTitle)
	fill := width - displayWidth(title) - 5
	if fill < 0 {
		fill = 0
	}
	return m.styles.ByToken("border").Render("╭─ ") +
		m.styles.ByToken(titleToken).Bold(true).Render(title) +
		m.styles.ByToken("border").Render(" "+strings.Repeat("─", fill)+"╮")
}

func (m *Model) boxBottom(width int) string {
	if width < 2 {
		return strings.Repeat(" ", maxInt(width, 0))
	}
	return m.styles.ByToken("border").Render("╰" + strings.Repeat("─", width-2) + "╯")
}

func (m *Model) boxContentLine(content []span, width int) string {
	innerWidth := width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}
	return m.styles.ByToken("border").Render("│") +
		m.renderSpans(content, innerWidth, "surface") +
		m.styles.ByToken("border").Render("│")
}

func (m *Model) composeLine(left, right []span, width int, background string) string {
	if width <= 0 {
		return ""
	}
	rightWidth := spansWidth(right)
	if rightWidth >= width {
		return m.renderSpans(right, width, background)
	}
	leftMax := width - rightWidth - 1
	left = fitSpans(left, leftMax)
	leftWidth := spansWidth(left)
	gap := width - leftWidth - rightWidth
	combined := append([]span{}, left...)
	if gap > 0 {
		combined = append(combined, span{text: strings.Repeat(" ", gap), token: "text"})
	}
	combined = append(combined, right...)
	return m.renderSpans(combined, width, background)
}

func (m *Model) renderSpans(spans []span, width int, background string) string {
	if width <= 0 {
		return ""
	}
	var builder strings.Builder
	used := 0
	for _, item := range spans {
		if used >= width {
			break
		}
		text := truncateDisplay(item.text, width-used)
		if text == "" {
			continue
		}
		style := m.styles.WithBackground(item.token, background)
		if item.bold {
			style = style.Bold(true)
		}
		builder.WriteString(style.Render(text))
		used += displayWidth(text)
	}
	if used < width {
		style := m.styles.WithBackground("text", background)
		builder.WriteString(style.Render(strings.Repeat(" ", width-used)))
	}
	return builder.String()
}

func composeSpans(left, right []span, width int) []span {
	rightWidth := spansWidth(right)
	if rightWidth >= width {
		return fitSpans(right, width)
	}
	left = fitSpans(left, width-rightWidth-1)
	leftWidth := spansWidth(left)
	gap := width - leftWidth - rightWidth
	combined := append([]span{}, left...)
	if gap > 0 {
		combined = append(combined, span{text: strings.Repeat(" ", gap), token: "text"})
	}
	combined = append(combined, right...)
	return combined
}

func fitSpans(spans []span, width int) []span {
	if width <= 0 {
		return nil
	}
	result := make([]span, 0, len(spans))
	used := 0
	for _, item := range spans {
		if used >= width {
			break
		}
		text := truncateDisplay(item.text, width-used)
		if text == "" {
			continue
		}
		item.text = text
		result = append(result, item)
		used += displayWidth(text)
	}
	return result
}

func spansWidth(spans []span) int {
	width := 0
	for _, item := range spans {
		width += displayWidth(item.text)
	}
	return width
}

func wrapText(text string, width int) []string {
	if width <= 0 || text == "" {
		return nil
	}
	words := strings.Fields(text)
	lines := make([]string, 0)
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if displayWidth(candidate) <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		current = truncateDisplay(word, width)
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func joinScreen(lines []string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	result := make([]string, 0, height)
	for index := 0; index < height; index++ {
		line := strings.Repeat(" ", width)
		if index < len(lines) {
			line = lines[index]
			if displayWidth(line) > width {
				line = lipgloss.NewStyle().MaxWidth(width).Render(line)
			}
			if padding := width - displayWidth(line); padding > 0 {
				line += strings.Repeat(" ", padding)
			}
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func truncateDisplay(text string, width int) string {
	if width <= 0 {
		return ""
	}
	var builder strings.Builder
	used := 0
	for _, runeValue := range text {
		runeWidth := displayWidth(string(runeValue))
		if used+runeWidth > width {
			break
		}
		builder.WriteRune(runeValue)
		used += runeWidth
	}
	return builder.String()
}

func padDisplay(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = truncateDisplay(text, width)
	padding := width - displayWidth(text)
	if padding <= 0 {
		return text
	}
	return text + strings.Repeat(" ", padding)
}

func displayWidth(text string) int {
	return lipgloss.Width(text)
}

func shorten(text string, width int) string {
	if text == "" {
		return "—"
	}
	return truncateDisplay(text, width)
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
