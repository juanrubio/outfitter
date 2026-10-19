/* Browser reference only. No live backend calls, shell commands, or disk writes. */
(() => {
  'use strict';
  const F = globalThis.OUTFITTER_FIXTURES;
  const $ = id => document.getElementById(id);
  const clone = x => JSON.parse(JSON.stringify(x));
  const clamp = (n, a, b) => Math.max(a, Math.min(b, n));
  const short = (s, n) => [...String(s)].length > n ? [...String(s)].slice(0, Math.max(0, n - 1)).join('') + '…' : String(s);
  const pad = (s, n) => short(s, n).padEnd(n, ' ');
  const esc = s => s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');

  const captions = {
    catalog: 'The catalog desk. Grouped categories, version diffs, backend tags, and live pending plan.',
    outdated: 'Bulk updates. Press u to queue all 5 outdated tools across vendor, mise:npm, mise:ubi, mise:aqua, and uv.',
    profiles: 'Preset profiles. Outfit a Full Desktop, Headless Ubuntu VPS, Harnesses Only, or Minimal rig in one key.',
    plan: 'Dry-run Plan & Apply. Inspect every command and sudo requirement before touching the system.',
    applying: 'Live execution progress. Per-tool status, elapsed time, and streaming command output.',
    bootstrap: 'Zero-prerequisite bootstrap. Outfitter downloads and verifies ~/.local/bin/mise automatically on a fresh Ubuntu box.',
    custom: 'Extensible catalog. Merge ~/.config/outfitter/catalog.toml overrides and local source builds seamlessly.',
    help: 'Keyboard navigation stays visible at all times. Browser controls are review framing only.'
  };

  let state, grid, regions = [], timer;

  function initial() {
    const tools = clone(F.tools).map(t => ({
      ...t,
      action: t.queuedAction || 'none' // 'none' | 'install' | 'update' | 'remove'
    }));
    return {
      w: 120,
      h: 40,
      theme: 'dark',
      view: 'catalog',
      scenario: 'catalog',
      selected: 'codex',
      filter: 'all', // 'all' | 'installed' | 'outdated' | 'pending' | 'uninstalled'
      query: '',
      editing: false,
      collapsed: {},
      profileId: 'desktop',
      profileCursor: 0,
      miseInstalled: true,
      scrollOffset: 0,
      notice: '',
      refreshing: false,
      applyStep: 2,
      tools
    };
  }

  class Grid {
    constructor(w, h) {
      this.w = w;
      this.h = h;
      this.cells = Array.from({ length: h }, () =>
        Array.from({ length: w }, () => [' ', 'text', 'canvas', false])
      );
    }
    text(x, y, text, fg = 'text', bg = null, bold = false, max = this.w - x) {
      if (y < 0 || y >= this.h) return;
      [...String(text)].slice(0, Math.max(0, max)).forEach((c, i) => {
        const xx = x + i;
        if (xx >= 0 && xx < this.w) {
          this.cells[y][xx] = [c, fg, c === '█' ? fg : bg || this.cells[y][xx][2], bold];
        }
      });
    }
    right(x, y, text, fg = 'text', bg = null, bold = false) {
      this.text(x - [...String(text)].length, y, text, fg, bg, bold);
    }
    fill(x, y, w, h, bg) {
      for (let yy = y; yy < y + h; yy++) {
        if (yy >= 0 && yy < this.h) {
          this.text(x, yy, ' '.repeat(Math.max(0, w)), 'text', bg);
        }
      }
    }
    line(x, y, w, fg = 'border', bg = null) {
      this.text(x, y, '─'.repeat(Math.max(0, w)), fg, bg);
    }
    vline(x, y, h, fg = 'border') {
      for (let i = 0; i < h; i++) this.text(x, y + i, '│', fg);
    }
    box(x, y, w, h, title = '', selected = false, bg = 'surface') {
      this.fill(x, y, w, h, bg);
      const c = selected ? 'accent' : 'border';
      this.text(x, y, '╭' + '─'.repeat(Math.max(0, w - 2)) + '╮', c, bg);
      for (let i = 1; i < h - 1; i++) {
        this.text(x, y + i, '│', c, bg);
        this.text(x + w - 1, y + i, '│', c, bg);
      }
      this.text(x, y + h - 1, '╰' + '─'.repeat(Math.max(0, w - 2)) + '╯', c, bg);
      if (title) {
        this.text(x + 2, y, short(' ' + title + ' ', w - 4), selected ? 'accent' : 'text', bg, true);
      }
    }
    render() {
      return this.cells
        .map(row => {
          let html = '', run = '', prev = '';
          const flush = () => {
            if (run) {
              const [fg, bg, b] = prev.split('|');
              html += `<span style="color:var(--${fg});background:var(--${bg});${b === '1' ? 'font-weight:700;' : ''}">${esc(run)}</span>`;
            }
          };
          for (const [c, fg, bg, bold] of row) {
            const key = `${fg}|${bg}|${bold ? 1 : 0}`;
            if (key !== prev) {
              flush();
              run = '';
              prev = key;
            }
            run += c;
          }
          flush();
          return html;
        })
        .join('\n');
    }
  }

  function region(x, y, w, h, action) {
    regions.push({ x, y, w, h, action });
  }

  function isOutdated(t) {
    return Boolean(t.installed && t.latest && t.installed !== t.latest);
  }

  function markerInfo(t) {
    if (t.action === 'install') return { mark: '[+]', fg: 'accent', label: 'Install queued' };
    if (t.action === 'update') return { mark: '[↑]', fg: 'accent', label: 'Update queued' };
    if (t.action === 'remove') return { mark: '[-]', fg: 'error', label: 'Remove queued' };
    if (isOutdated(t)) return { mark: '[!]', fg: 'low', label: 'Update available' };
    if (t.installed) return { mark: '[✓]', fg: 'good', label: 'Installed (current)' };
    return { mark: '[ ]', fg: 'muted', label: 'Not installed' };
  }

  function counts() {
    const total = state.tools.length;
    const installed = state.tools.filter(t => t.installed).length;
    const outdated = state.tools.filter(isOutdated).length;
    const add = state.tools.filter(t => t.action === 'install').length;
    const up = state.tools.filter(t => t.action === 'update').length;
    const rm = state.tools.filter(t => t.action === 'remove').length;
    return { total, installed, outdated, add, up, rm, pending: add + up + rm };
  }

  function filteredTools() {
    return state.tools.filter(t => {
      if (state.filter === 'installed' && !t.installed) return false;
      if (state.filter === 'outdated' && !isOutdated(t)) return false;
      if (state.filter === 'pending' && t.action === 'none') return false;
      if (state.filter === 'uninstalled' && t.installed) return false;
      if (state.query) {
        const q = state.query.toLowerCase();
        const hay = `${t.name} ${t.binary} ${t.backend} ${t.package} ${t.description}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  }

  function selectedTool() {
    const xs = filteredTools();
    return xs.find(t => t.id === state.selected) || state.tools.find(t => t.id === state.selected) || state.tools[0];
  }

  function toggleTool(t) {
    if (!t) return;
    if (!t.installed) {
      t.action = t.action === 'install' ? 'none' : 'install';
    } else if (isOutdated(t)) {
      if (t.action === 'none') t.action = 'update';
      else if (t.action === 'update') t.action = 'remove';
      else t.action = 'none';
    } else {
      t.action = t.action === 'remove' ? 'none' : 'remove';
    }
    state.notice = `${t.name}: ${markerInfo(t).label}`;
  }

  function queueAllUpdates() {
    let n = 0;
    state.tools.forEach(t => {
      if (isOutdated(t) && t.action !== 'update') {
        t.action = 'update';
        n++;
      }
    });
    state.notice = n ? `Queued ${n} updates` : 'All outdated tools already queued';
  }

  function toggleCategory(catId) {
    const catTools = state.tools.filter(t => t.category === catId);
    const anyUnqueued = catTools.some(t => (!t.installed && t.action !== 'install') || (isOutdated(t) && t.action !== 'update'));
    catTools.forEach(t => {
      if (anyUnqueued) {
        if (!t.installed) t.action = 'install';
        else if (isOutdated(t)) t.action = 'update';
      } else {
        t.action = 'none';
      }
    });
    state.notice = anyUnqueued ? `Queued category ${catId}` : `Cleared queued changes in ${catId}`;
  }

  function applyProfile(profId) {
    const p = F.profiles.find(x => x.id === profId) || F.profiles[0];
    state.profileId = p.id;
    const set = new Set(p.tools);
    let added = 0;
    state.tools.forEach(t => {
      if (set.has(t.id)) {
        if (!t.installed) {
          t.action = 'install';
          added++;
        } else if (isOutdated(t)) {
          t.action = 'update';
        } else {
          t.action = 'none';
        }
      } else if (!t.installed) {
        t.action = 'none';
      }
    });
    state.notice = `Profile '${p.name}' loaded (+${added} to install)`;
  }

  function wrapLines(str, width) {
    const words = String(str).split(/\s+/);
    const lines = [];
    let cur = '';
    for (const w of words) {
      if (!cur) cur = w;
      else if (cur.length + 1 + w.length <= width) cur += ' ' + w;
      else {
        lines.push(cur);
        cur = w;
      }
    }
    if (cur) lines.push(cur);
    return lines;
  }

  function drawHeader() {
    const c = counts();
    const prof = F.profiles.find(p => p.id === state.profileId) || F.profiles[0];
    grid.text(2, 1, 'OUTFITTER', 'accent', null, true);
    if (state.w >= 96) {
      const miseBadge = state.miseInstalled
        ? `mise ${F.mise.version} ready`
        : 'mise missing (auto-bootstrap queued)';
      grid.text(13, 1, `Profile: ${prof.name} · ${miseBadge}`, state.miseInstalled ? 'muted' : 'low');
    }
    const planSummary = c.pending
      ? `Plan: +${c.add} ↑${c.up} -${c.rm}`
      : 'Plan: clean';
    const rightTop = state.w >= 80
      ? `${c.installed}/${c.total} installed · ${c.outdated} outdated · ${planSummary}`
      : `${c.installed}/${c.total} · +${c.add} ↑${c.up}`;
    grid.right(state.w - 2, 1, rightTop, c.pending ? 'accent' : 'muted', null, Boolean(c.pending));

    const filterLabel = {
      all: `All (${c.total})`,
      installed: `Installed (${c.installed})`,
      outdated: `Outdated (${c.outdated})`,
      pending: `Pending (${c.pending})`,
      uninstalled: `Not installed (${c.total - c.installed})`
    }[state.filter];

    grid.text(2, 3, 'Filter:', 'muted');
    grid.text(10, 3, `[ ${filterLabel} ]`, 'accent', null, true);
    region(10, 3, filterLabel.length + 4, 1, () => {
      const order = ['all', 'outdated', 'pending', 'installed', 'uninstalled'];
      state.filter = order[(order.indexOf(state.filter) + 1) % order.length];
      render();
    });

    if (state.w >= 92) {
      grid.text(30, 3, '[p] Profile', 'text');
      region(30, 3, 11, 1, () => { state.view = 'profiles'; render(); });
      grid.text(44, 3, `[u] Queue ${c.outdated} updates`, c.outdated ? 'low' : 'muted');
      region(44, 3, 20, 1, () => { queueAllUpdates(); render(); });
      grid.text(67, 3, `[a] Apply plan (${c.pending})`, c.pending ? 'good' : 'muted', null, Boolean(c.pending));
      region(67, 3, 19, 1, () => { state.view = 'plan'; render(); });
    }
    if (state.w >= 110) {
      grid.right(state.w - 2, 3, state.refreshing ? 'Checking upstream…' : state.notice || 'Ubuntu 24.04 · amd64', state.refreshing ? 'accent' : 'muted');
    }
    grid.line(2, 4, state.w - 4);
  }

  function drawFooter(text, pos = '') {
    grid.line(2, state.h - 3, state.w - 4);
    grid.text(2, state.h - 2, short(text, state.w - 4 - (pos ? pos.length + 2 : 0)), 'muted');
    if (pos) grid.right(state.w - 2, state.h - 2, pos, 'accent');
  }

  function buildListRows(xs) {
    const rows = [];
    for (const cat of F.categories) {
      const catAll = state.tools.filter(t => t.category === cat.id);
      const catVisible = xs.filter(t => t.category === cat.id);
      if (!catVisible.length && (state.filter !== 'all' || state.query)) continue;
      const instCount = catAll.filter(t => t.installed).length;
      const upCount = catAll.filter(isOutdated).length;
      const isCol = Boolean(state.collapsed[cat.id]);
      rows.push({
        type: 'category',
        cat,
        instCount,
        totalCount: catAll.length,
        upCount,
        collapsed: isCol
      });
      if (!isCol) {
        for (const tool of catVisible) {
          rows.push({ type: 'tool', tool });
        }
      }
    }
    return rows;
  }

  function drawInspector(x, y, w, h, t) {
    const c = counts();
    const planBoxH = h >= 28 ? 9 : 7;
    const detailH = h - planBoxH - 1;

    grid.box(x, y, w, detailH, `INSPECTOR · ${t.binary}`, false);
    const innerW = w - 4;
    let ry = y + 2;

    grid.text(x + 2, ry, short(t.name, innerW - 12), 'text', 'surface', true);
    grid.right(x + w - 2, ry, t.backend + (t.sudo ? ' [sudo]' : ''), t.sudo ? 'low' : 'accent', 'surface', true);
    ry += 1;

    const descLines = wrapLines(t.description, innerW).slice(0, 2);
    descLines.forEach(line => {
      grid.text(x + 2, ry++, short(line, innerW), 'muted', 'surface');
    });
    ry += 1;

    const mi = markerInfo(t);
    const kv = [
      ['State', `${mi.mark} ${mi.label}`, mi.fg],
      ['Installed', t.installed || '— (not installed)', t.installed ? 'good' : 'muted'],
      ['Latest', t.latest + (isOutdated(t) ? ' (update available)' : ''), isOutdated(t) ? 'low' : 'text'],
      ['Binary path', t.path || '~/.local/bin/' + t.binary, 'muted'],
      ['Package', t.package, 'text'],
      ['Provenance', t.provenance, 'muted']
    ];

    for (const [k, v, fg] of kv) {
      if (ry >= y + detailH - 5) break;
      grid.text(x + 2, ry, pad(k, 12), 'muted', 'surface');
      grid.text(x + 15, ry, short(v, innerW - 13), fg, 'surface', k === 'State');
      ry++;
    }

    const profNames = F.profiles.filter(p => p.tools.includes(t.id)).map(p => p.short).join(' · ');
    if (ry < y + detailH - 4) {
      grid.text(x + 2, ry, pad('Profiles', 12), 'muted', 'surface');
      grid.text(x + 15, ry, short(profNames || 'Custom', innerW - 13), 'text', 'surface');
      ry += 2;
    }

    if (ry < y + detailH - 2) {
      const cmdLabel = t.action === 'remove' ? 'Uninstall cmd' : (t.installed ? 'Update cmd' : 'Install cmd');
      const cmdStr = t.action === 'remove' ? t.removeCmd : (t.installed ? t.updateCmd : t.installCmd);
      grid.text(x + 2, ry++, cmdLabel.toUpperCase(), 'muted', 'surface', true);
      if (ry < y + detailH - 1) {
        grid.text(x + 2, ry++, short('$ ' + cmdStr, innerW), 'accent', 'surface');
      }
    }

    // Bottom Right: Pending Action Plan Box
    const py = y + detailH + 1;
    grid.box(x, py, w, planBoxH, `PENDING ACTION PLAN (${c.pending})`, c.pending > 0);
    const adds = state.tools.filter(i => i.action === 'install').map(i => i.binary);
    const ups = state.tools.filter(i => i.action === 'update').map(i => i.binary);
    const rms = state.tools.filter(i => i.action === 'remove').map(i => i.binary);

    let pry = py + 2;
    if (!state.miseInstalled && pry < py + planBoxH - 2) {
      grid.text(x + 2, pry++, short('★ Bootstrap ~/.local/bin/mise v' + F.mise.version, innerW), 'low', 'surface', true);
    }
    grid.text(x + 2, pry++, short(`+ Install (${adds.length}): ${adds.length ? adds.join(', ') : 'none'}`, innerW), adds.length ? 'accent' : 'muted', 'surface');
    grid.text(x + 2, pry++, short(`↑ Update  (${ups.length}): ${ups.length ? ups.join(', ') : 'none'}`, innerW), ups.length ? 'low' : 'muted', 'surface');
    if (pry < py + planBoxH - 2) {
      grid.text(x + 2, pry++, short(`- Remove  (${rms.length}): ${rms.length ? rms.join(', ') : 'none'}`, innerW), rms.length ? 'error' : 'muted', 'surface');
    }
    grid.text(x + 2, py + planBoxH - 2, short('Press [a] to review & apply · [x] reset plan', innerW), c.pending ? 'good' : 'muted', 'surface');
    region(x, py, w, planBoxH, () => { state.view = 'plan'; render(); });
  }

  function drawCatalog() {
    drawHeader();
    const xs = filteredTools();
    if (!xs.some(t => t.id === state.selected) && xs.length) {
      state.selected = xs[0].id;
    }
    const sel = selectedTool();

    let listY = 6;
    if (state.query || state.editing) {
      grid.text(2, 5, short(`/ ${state.query}${state.editing ? '▏' : ''}  (Esc to clear)`, state.w - 4), 'accent');
    } else if (!state.miseInstalled) {
      grid.text(2, 5, short('! mise is not installed on this machine. Outfitter will auto-bootstrap ~/.local/bin/mise (SHA-256 verified) on Apply [a] or [b].', state.w - 4), 'low', null, true);
    }

    const wide = state.w >= 100 && state.h >= 24;
    const listW = wide ? Math.floor((state.w - 6) * 0.56) : state.w - 4;
    const listH = state.h - listY - 3;

    const rows = buildListRows(xs);
    const selRowIdx = Math.max(0, rows.findIndex(r => r.type === 'tool' && r.tool.id === state.selected));
    if (selRowIdx < state.scrollOffset) state.scrollOffset = selRowIdx;
    if (selRowIdx >= state.scrollOffset + listH) state.scrollOffset = selRowIdx - listH + 1;
    state.scrollOffset = clamp(state.scrollOffset, 0, Math.max(0, rows.length - listH));

    const visibleRows = rows.slice(state.scrollOffset, state.scrollOffset + listH);

    visibleRows.forEach((r, idx) => {
      const yy = listY + idx;
      if (r.type === 'category') {
        const arrow = r.collapsed ? '▸' : '▾';
        const upNote = r.upCount ? ` · ${r.upCount} outdated` : '';
        const hdr = `${arrow} ${r.cat.num} · ${r.cat.title} (${r.instCount}/${r.totalCount}${upNote})`;
        grid.text(2, yy, short(hdr, listW), 'text', null, true);
        region(2, yy, listW, 1, () => {
          state.collapsed[r.cat.id] = !state.collapsed[r.cat.id];
          render();
        });
      } else {
        const t = r.tool;
        const active = t.id === state.selected;
        const rowBg = active ? 'surface' : 'canvas';
        if (active) grid.fill(2, yy, listW, 1, 'surface');

        const mi = markerInfo(t);
        grid.text(2, yy, active ? '>' : ' ', active ? 'accent' : 'muted', rowBg, true);
        grid.text(4, yy, mi.mark, mi.fg, rowBg, true);

        const nameW = clamp(listW - 34, 14, 24);
        grid.text(8, yy, pad(t.name, nameW), active ? 'accent' : (t.installed ? 'text' : 'muted'), rowBg, active);

        let verText = '—';
        let verFg = 'muted';
        if (t.action === 'install') {
          verText = `— → ${t.latest}`;
          verFg = 'accent';
        } else if (t.action === 'update' || isOutdated(t)) {
          verText = `${short(t.installed, 8)} → ${short(t.latest, 8)}`;
          verFg = t.action === 'update' ? 'accent' : 'low';
        } else if (t.installed) {
          verText = short(t.installed, 15);
          verFg = 'good';
        } else {
          verText = `— (${short(t.latest, 8)})`;
          verFg = 'muted';
        }

        const verCol = 8 + nameW + 1;
        grid.text(verCol, yy, pad(verText, 19), verFg, rowBg, t.action !== 'none');

        if (listW >= 54) {
          grid.right(2 + listW - 1, yy, t.backend, t.backend === 'apt' ? 'low' : 'muted', rowBg);
        }

        region(2, yy, listW, 1, () => {
          if (state.selected === t.id) toggleTool(t);
          else state.selected = t.id;
          render();
        });
      }
    });

    if (wide && sel) {
      const rightX = 2 + listW + 2;
      const rightW = state.w - rightX - 2;
      drawInspector(rightX, listY, rightW, listH, sel);
    }

    const pos = rows.length > listH ? `${selRowIdx + 1}/${rows.length}` : `${ state.tools.length } tools`;
    const helpHint = state.w >= 100
      ? '↑↓/jk Move  Space Toggle  u Queue updates  g Group  p Profile  f Filter  / Search  a Apply  ? Help'
      : 'jk Move  Space Toggle  Enter Inspector  u Updates  p Profile  a Apply  ? Help';
    drawFooter(helpHint, pos);
  }

  function drawDetailCard() {
    drawHeader('Tool Inspector · Esc returns to catalog');
    const t = selectedTool();
    drawInspector(2, 6, state.w - 4, state.h - 10, t);
    drawFooter('Space Toggle action  u Queue updates  a Plan & Apply  Esc Back to catalog');
  }

  function drawProfilesModal() {
    drawCatalog();
    const mw = Math.min(state.w - 6, 86);
    const mh = Math.min(state.h - 6, 22);
    const mx = Math.floor((state.w - mw) / 2);
    const my = Math.floor((state.h - mh) / 2);

    grid.box(mx, my, mw, mh, 'SELECT OUTFITTER PROFILE (p)', true);
    grid.text(mx + 3, my + 2, 'Choose a preset bundle to queue missing tools for this machine:', 'muted', 'surface');

    F.profiles.forEach((p, idx) => {
      const yy = my + 4 + idx * 3;
      if (yy + 2 >= my + mh - 2) return;
      const active = idx === state.profileCursor;
      const isCurrent = p.id === state.profileId;
      grid.text(mx + 3, yy, active ? '> ' : '  ', 'accent', 'surface', true);
      grid.text(mx + 5, yy, `${idx + 1}. ${p.name}`, active ? 'accent' : 'text', 'surface', true);
      grid.right(mx + mw - 3, yy, `${p.tools.length} tools${isCurrent ? ' [active]' : ''}`, isCurrent ? 'good' : 'muted', 'surface');
      grid.text(mx + 8, yy + 1, short(p.description, mw - 11), 'muted', 'surface');
      region(mx + 2, yy, mw - 4, 2, () => {
        state.profileCursor = idx;
        applyProfile(p.id);
        state.view = 'catalog';
        render();
      });
    });

    grid.text(mx + 3, my + mh - 2, '↑↓/jk Select   Enter Apply profile to plan   Esc Cancel', 'accent', 'surface');
  }

  function drawPlanModal() {
    drawHeader('Plan & Apply · Dry-Run Review');
    const boxX = 2, boxY = 6, boxW = state.w - 4, boxH = state.h - 10;
    grid.box(boxX, boxY, boxW, boxH, 'DRY-RUN EXECUTION PLAN (Nothing has been modified on disk yet)', true);

    const adds = state.tools.filter(t => t.action === 'install');
    const ups = state.tools.filter(t => t.action === 'update');
    const rms = state.tools.filter(t => t.action === 'remove');
    const needsSudo = [...adds, ...ups, ...rms].some(t => t.sudo);

    let ry = boxY + 2;
    const innerW = boxW - 4;

    if (!adds.length && !ups.length && !rms.length && state.miseInstalled) {
      grid.text(boxX + 3, ry + 2, 'No pending changes in your plan.', 'text', 'surface', true);
      grid.text(boxX + 3, ry + 4, 'Press Space on any tool in the catalog, or press [u] to queue all 5 available updates.', 'muted', 'surface');
      drawFooter('u Queue all updates   Esc Return to catalog');
      return;
    }

    grid.text(boxX + 3, ry++, short(`Summary: +${adds.length} install · ↑${ups.length} update · -${rms.length} remove${needsSudo ? ' · [sudo required for apt]' : ' · [user-space only, no sudo]'}`, innerW), 'text', 'surface', true);
    ry++;

    if (!state.miseInstalled && ry < boxY + boxH - 4) {
      grid.text(boxX + 3, ry++, '0. BOOTSTRAP PREREQUISITE (mise)', 'low', 'surface', true);
      grid.text(boxX + 5, ry++, short('$ curl -fsSL https://mise.jdx.dev/mise-v2026.10.1-linux-x64.tar.gz | sha256sum -c && install ~/.local/bin/mise', innerW - 2), 'accent', 'surface');
      ry++;
    }

    const sections = [
      { title: `INSTALLATIONS (+${adds.length})`, items: adds, fg: 'accent', sym: '+', cmdKey: 'installCmd' },
      { title: `UPDATES (↑${ups.length})`, items: ups, fg: 'low', sym: '↑', cmdKey: 'updateCmd' },
      { title: `REMOVALS (-${rms.length})`, items: rms, fg: 'error', sym: '-', cmdKey: 'removeCmd' }
    ];

    for (const sec of sections) {
      if (!sec.items.length || ry >= boxY + boxH - 3) continue;
      grid.text(boxX + 3, ry++, sec.title, sec.fg, 'surface', true);
      for (const t of sec.items) {
        if (ry >= boxY + boxH - 3) break;
        const verInfo = sec.sym === '+' ? t.latest : (sec.sym === '↑' ? `${t.installed} → ${t.latest}` : t.installed);
        grid.text(boxX + 5, ry, `${sec.sym} ${pad(t.name, 20)} ${pad(verInfo, 22)} [${t.backend}${t.sudo ? '·sudo' : ''}]`, 'text', 'surface');
        if (boxW >= 108) {
          grid.text(boxX + 64, ry, short('$ ' + t[sec.cmdKey], boxW - 66), 'muted', 'surface');
        }
        ry++;
      }
      ry++;
    }

    grid.text(boxX + 3, boxY + boxH - 2, 'Enter / y Confirm and execute plan     e Export shell script     Esc Cancel', 'good', 'surface', true);
    drawFooter('Enter Execute plan   u Queue all updates   Esc Return to catalog');
  }

  function drawApplyingScreen() {
    drawHeader('Executing Action Plan · 4 of 6 completed');
    const boxX = 2, boxY = 6, boxW = state.w - 4, boxH = state.h - 10;
    grid.box(boxX, boxY, boxW, boxH, 'LIVE EXECUTION PROGRESS', true);
    const innerW = boxW - 6;

    // Progress bar
    const barW = Math.min(48, innerW - 24);
    const filled = Math.round(barW * (4 / 6));
    grid.text(boxX + 3, boxY + 2, 'Overall progress  ', 'muted', 'surface');
    grid.text(boxX + 21, boxY + 2, '█'.repeat(filled), 'good', 'surface');
    grid.text(boxX + 21 + filled, boxY + 2, '░'.repeat(barW - filled), 'border', 'surface');
    grid.text(boxX + 23 + barW, boxY + 2, '67% (4/6 steps · 0 failed)', 'accent', 'surface', true);

    const steps = [
      { state: 'done', name: 'OpenAI Codex CLI', detail: '0.163.0 → 0.165.0', backend: 'mise:npm', time: '1.8s', cmd: 'mise upgrade npm:@openai/codex' },
      { state: 'done', name: 'Trailmark', detail: '0.3.0 → 0.3.1', backend: 'mise:ubi', time: '1.1s', cmd: 'mise upgrade ubi:juanrubio/trailmark' },
      { state: 'done', name: 'Lazygit', detail: '0.44.0 → 0.45.1', backend: 'mise:aqua', time: '0.9s', cmd: 'mise upgrade aqua:jesseduffield/lazygit' },
      { state: 'done', name: 'Gemini CLI', detail: '— → 0.43.0', backend: 'mise:npm', time: '2.4s', cmd: 'mise use --global npm:@google/gemini-cli@latest' },
      { state: 'run',  name: 'Muse Code', detail: '1.4.10-R6427.1 → 1.4.12-R6501.0', backend: 'vendor', time: '3.2s…', cmd: 'MUSE_UPDATE_INTERVAL_SECONDS=0 muse --version' },
      { state: 'wait', name: 'Quotary', detail: '— → 0.1.0-dev', backend: 'source', time: 'queued', cmd: 'go -C ~/repos/quotary build -o ~/.local/bin/quotary ./cmd/quotary' }
    ];

    let ry = boxY + 4;
    for (const s of steps) {
      if (ry >= boxY + boxH - 9) break;
      const icon = s.state === 'done' ? '✓' : (s.state === 'run' ? '⠋' : '·');
      const fg = s.state === 'done' ? 'good' : (s.state === 'run' ? 'accent' : 'muted');
      grid.text(boxX + 3, ry, `${icon} ${pad(s.name, 20)} ${pad(s.detail, 32)} [${pad(s.backend, 9)}]`, fg, 'surface', s.state === 'run');
      grid.right(boxX + boxW - 3, ry, s.time, fg, 'surface');
      ry++;
    }

    // Live log tail box
    const logY = boxY + boxH - 8;
    grid.line(boxX + 2, logY, boxW - 4, 'border', 'surface');
    grid.text(boxX + 3, logY + 1, 'LIVE OUTPUT · Muse Code (vendor self-updater)', 'accent', 'surface', true);
    const logs = [
      '$ MUSE_UPDATE_INTERVAL_SECONDS=0 ~/.local/bin/muse --version',
      '[muse-launcher] Checking channel https://api.meta.ai/muse-code/channels/muse-stable...',
      '[muse-launcher] Downloading muse-bin-1.4.12-R6501.0 from lookaside.facebook.com (84%)...',
      '[muse-launcher] Verifying SHA-256 digest and updating ~/.local/bin/.muse-version'
    ];
    logs.forEach((l, i) => {
      if (logY + 2 + i < boxY + boxH - 1) {
        grid.text(boxX + 3, logY + 2 + i, short(l, innerW), i === logs.length - 1 ? 'text' : 'muted', 'surface');
      }
    });

    drawFooter('Esc Return to catalog (background task continues safely)   l Full log file');
  }

  function drawHelpModal() {
    drawHeader('Keyboard Reference');
    const boxX = 2, boxY = 6, boxW = state.w - 4, boxH = state.h - 10;
    grid.box(boxX, boxY, boxW, boxH, 'OUTFITTER KEYBOARD SHORTCUTS (?)', true);

    const pairs = [
      ['↑ / ↓ or j / k', 'Move selection between tools in the catalog'],
      ['1 / 2 / 3 / 4', 'Jump directly to Category 1 (Harnesses), 2 (AI TUIs), 3 (Terminal), 4 (Utilities)'],
      ['Tab', 'Collapse or expand the currently focused category'],
      ['Space', 'Toggle desired action for selected tool ([ ] ↔ [+], [!] → [↑] → [-], [✓] ↔ [-])'],
      ['u', 'Queue updates ([↑]) for ALL outdated tools in the catalog at once'],
      ['g', 'Toggle all tools in the focused category'],
      ['p', 'Open Preset Profile selector (Full Desktop, Headless Server, Harnesses, Minimal)'],
      ['a', 'Open Plan & Apply dry-run confirmation screen'],
      ['x', 'Reset/clear all pending actions in the current plan'],
      ['f', 'Cycle filter: All → Outdated → Pending → Installed → Not Installed'],
      ['/', 'Incremental search by tool name, binary, backend, or description'],
      ['b', 'Simulate missing/installed mise bootstrap state'],
      ['r', 'Refresh installed binary checks and upstream version readings'],
      ['Esc / q', 'Close active overlay or return to the catalog desk']
    ];

    let ry = boxY + 2;
    for (const [k, desc] of pairs) {
      if (ry >= boxY + boxH - 2) break;
      grid.text(boxX + 4, ry, pad(k, 18), 'accent', 'surface', true);
      grid.text(boxX + 24, ry, short(desc, boxW - 27), 'text', 'surface');
      ry++;
    }
    drawFooter('Esc or ? Close help');
  }

  function applyScenario(sc) {
    state = initial();
    state.scenario = sc;
    const [w, h] = $('size').value.split('×').map(Number);
    state.w = w;
    state.h = h;
    state.theme = $('theme').value;

    if (sc === 'outdated') {
      state.filter = 'outdated';
      queueAllUpdates();
      state.selected = 'codex';
    } else if (sc === 'profiles') {
      state.view = 'profiles';
      state.profileCursor = 1; // Server / VPS highlighted
    } else if (sc === 'plan') {
      queueAllUpdates();
      state.view = 'plan';
    } else if (sc === 'applying') {
      queueAllUpdates();
      state.view = 'applying';
    } else if (sc === 'bootstrap') {
      state.miseInstalled = false;
      state.selected = 'boardwalk';
    } else if (sc === 'custom') {
      state.selected = 'quotary';
      state.notice = 'Loaded ~/.config/outfitter/catalog.toml (1 override)';
    } else if (sc === 'help') {
      state.view = 'help';
    }
    render();
  }

  function fitStage() {
    const frame = $('terminal-frame');
    const stage = $('stage');
    if (!frame || !stage) return;
    frame.style.transform = 'none';
    const avail = stage.clientWidth || window.innerWidth - 72;
    const natural = frame.offsetWidth;
    const scale = avail < natural ? Math.max(0.5, avail / natural) : 1;
    frame.style.transform = scale < 1 ? `scale(${scale})` : 'none';
    stage.style.height = `${Math.ceil(frame.offsetHeight * scale)}px`;
    const note = $('scale-note');
    if (note) note.textContent = scale < 1 ? `Scaled to ${Math.round(scale * 100)}% to fit viewport` : '100% scale · exact character grid';
  }

  function render() {
    regions = [];
    grid = new Grid(state.w, state.h);
    $('terminal-frame').dataset.theme = state.theme === 'dark' ? '' : state.theme;
    $('dimensions').textContent = `${state.w} columns × ${state.h} rows`;
    $('caption').textContent = captions[state.view === 'catalog' ? state.scenario : state.view] || captions.catalog;

    if (state.view === 'profiles') drawProfilesModal();
    else if (state.view === 'plan') drawPlanModal();
    else if (state.view === 'applying') drawApplyingScreen();
    else if (state.view === 'help') drawHelpModal();
    else if (state.view === 'detail') drawDetailCard();
    else drawCatalog();

    $('terminal').innerHTML = grid.render();
    fitStage();
  }

  function moveSelection(delta) {
    const xs = filteredTools().filter(t => !state.collapsed[t.category]);
    if (!xs.length) return;
    const idx = Math.max(0, xs.findIndex(t => t.id === state.selected));
    const next = xs[clamp(idx + delta, 0, xs.length - 1)];
    if (next) state.selected = next.id;
  }

  function onKey(e) {
    if (['ArrowUp', 'ArrowDown', 'Space', ' ', 'Tab'].includes(e.key)) e.preventDefault();

    if (state.editing) {
      if (e.key === 'Escape' || e.key === 'Enter') {
        state.editing = false;
        if (e.key === 'Escape') state.query = '';
      } else if (e.key === 'Backspace') {
        state.query = state.query.slice(0, -1);
      } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey) {
        state.query += e.key;
      }
      render();
      return;
    }

    if (state.view === 'profiles') {
      if (e.key === 'Escape' || e.key === 'p' || e.key === 'q') state.view = 'catalog';
      else if (e.key === 'ArrowDown' || e.key === 'j') state.profileCursor = (state.profileCursor + 1) % F.profiles.length;
      else if (e.key === 'ArrowUp' || e.key === 'k') state.profileCursor = (state.profileCursor + F.profiles.length - 1) % F.profiles.length;
      else if (e.key === 'Enter') {
        applyProfile(F.profiles[state.profileCursor].id);
        state.view = 'catalog';
      }
      render();
      return;
    }

    if (state.view === 'plan') {
      if (e.key === 'Escape' || e.key === 'n' || e.key === 'q') state.view = 'catalog';
      else if (e.key === 'Enter' || e.key === 'y') state.view = 'applying';
      else if (e.key === 'u') queueAllUpdates();
      render();
      return;
    }

    if (state.view === 'applying' || state.view === 'help' || state.view === 'detail') {
      if (e.key === 'Escape' || e.key === 'q' || e.key === '?') state.view = 'catalog';
      else if (state.view === 'detail' && e.key === ' ') toggleTool(selectedTool());
      render();
      return;
    }

    // Catalog view keys
    if (e.key === 'ArrowDown' || e.key === 'j') moveSelection(1);
    else if (e.key === 'ArrowUp' || e.key === 'k') moveSelection(-1);
    else if (e.key === ' ') toggleTool(selectedTool());
    else if (e.key === 'u') queueAllUpdates();
    else if (e.key === 'g') {
      const t = selectedTool();
      if (t) toggleCategory(t.category);
    } else if (e.key === 'x') {
      state.tools.forEach(t => { t.action = 'none'; });
      state.notice = 'Cleared all pending plan actions';
    } else if (e.key === 'p') state.view = 'profiles';
    else if (e.key === 'a') state.view = 'plan';
    else if (e.key === '?') state.view = 'help';
    else if (e.key === '/') {
      state.editing = true;
    } else if (e.key === 'f') {
      const order = ['all', 'outdated', 'pending', 'installed', 'uninstalled'];
      state.filter = order[(order.indexOf(state.filter) + 1) % order.length];
    } else if (e.key === 'b') {
      state.miseInstalled = !state.miseInstalled;
      state.notice = state.miseInstalled ? 'mise marked installed' : 'Simulated missing mise (auto-bootstrap active)';
    } else if (e.key === 'Tab') {
      const t = selectedTool();
      if (t) state.collapsed[t.category] = !state.collapsed[t.category];
    } else if (['1', '2', '3', '4'].includes(e.key)) {
      const cat = F.categories[Number(e.key) - 1];
      if (cat) {
        state.collapsed[cat.id] = false;
        const first = filteredTools().find(t => t.category === cat.id);
        if (first) state.selected = first.id;
      }
    } else if (e.key === 'Enter') {
      if (state.w < 100) state.view = 'detail';
      else state.view = 'plan';
    } else if (e.key === 'r' || e.key === 'R') {
      state.refreshing = true;
      clearTimeout(timer);
      timer = setTimeout(() => {
        state.refreshing = false;
        state.notice = 'Upstream versions verified just now';
        render();
      }, 450);
    }
    render();
  }

  function init() {
    state = initial();
    $('screen').addEventListener('change', e => applyScenario(e.target.value));
    $('size').addEventListener('change', e => {
      const [w, h] = e.target.value.split('×').map(Number);
      state.w = w;
      state.h = h;
      render();
    });
    $('theme').addEventListener('change', e => {
      state.theme = e.target.value;
      render();
    });
    $('queue-updates').addEventListener('click', () => {
      queueAllUpdates();
      render();
      $('terminal').focus();
    });
    $('reset').addEventListener('click', () => {
      $('screen').value = 'catalog';
      applyScenario('catalog');
      $('terminal').focus();
    });
    $('terminal').addEventListener('keydown', onKey);
    $('terminal').addEventListener('click', e => {
      const rect = $('terminal').getBoundingClientRect();
      const scale = rect.width / (state.w * 9 || rect.width);
      const cellW = rect.width / state.w;
      const cellH = rect.height / state.h;
      const cx = Math.floor((e.clientX - rect.left) / cellW);
      const cy = Math.floor((e.clientY - rect.top) / cellH);
      const hit = regions.find(r => cx >= r.x && cx < r.x + r.w && cy >= r.y && cy < r.y + r.h);
      if (hit) hit.action();
      $('terminal').focus();
    });
    window.addEventListener('resize', fitStage);
    render();
  }

  window.addEventListener('DOMContentLoaded', init);
})();
