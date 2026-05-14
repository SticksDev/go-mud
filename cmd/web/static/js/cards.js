const pendingGroups = {};
const earlyStatuses = {};

function renderStatusHTML(state, count, progress) {
  const countHTML = count > 1 ? '<span class="cmd-count">' + count + ' cmds</span>' : '';
  if (state === 'queued') return countHTML + '<span class="queued-label">Queued</span>';
  if (state === 'executing') {
    const progHTML = progress ? '<span class="progress-label">' + progress + '</span>' : '';
    return countHTML + progHTML + '<div class="spinner"></div>';
  }
  return countHTML + '<span class="queued-label">Queued</span>';
}

function addSentCard(id, text) {
  clearEmpty();

  const group = document.createElement('div');
  group.className = 'request-group';

  const card = document.createElement('div');
  card.className = 'card-sent';

  const cmds = parseCommands(text);

  const row = document.createElement('div');
  row.className = 'cmd-row';

  const cmdBlock = document.createElement('div');
  cmdBlock.className = 'cmd-list';
  cmds.forEach(c => {
    const line = document.createElement('div');
    line.className = 'cmd-text';
    line.textContent = '> ' + c;
    cmdBlock.appendChild(line);
  });

  const statusEl = document.createElement('span');
  statusEl.className = 'cmd-status';

  const buffered = earlyStatuses[id];
  delete earlyStatuses[id];
  const initialState = buffered || 'queued';
  statusEl.innerHTML = renderStatusHTML(initialState, cmds.length, null);

  row.append(cmdBlock, statusEl);
  card.appendChild(row);
  group.appendChild(card);

  const resultsSlot = document.createElement('div');
  group.appendChild(resultsSlot);

  logContainer.appendChild(group);
  scrollToBottom();

  if (initialState === 'executing') startLoadingSound();
  playSubmit();

  pendingGroups[id] = { group, card, statusEl, resultsSlot, count: cmds.length, progress: null };
}

function updateSentCard(id, state) {
  const entry = pendingGroups[id];
  if (!entry) {
    earlyStatuses[id] = state;
    return;
  }
  entry.statusEl.innerHTML = renderStatusHTML(state, entry.count, entry.progress);
  if (state === 'executing') startLoadingSound();
}

function updateProgress(id, current, total, command) {
  const entry = pendingGroups[id];
  if (!entry) return;
  entry.progress = current + '/' + total;
  entry.statusEl.innerHTML = renderStatusHTML('executing', entry.count, entry.progress);
}

function resolveSentCard(id, durationMs, hasError) {
  const entry = pendingGroups[id];
  if (!entry) return;

  stopLoadingSound();
  if (hasError) playError(); else playSuccess();

  const icon = hasError ? svgError : svgCheck;
  const dur = fmtDuration(durationMs);
  const countHTML = entry.count > 1 ? '<span class="cmd-count">' + entry.count + ' cmds</span>' : '';
  entry.statusEl.innerHTML = countHTML + icon + (dur ? '<span class="cmd-duration">' + dur + '</span>' : '');
  entry.card.style.borderLeftColor = hasError ? 'var(--red)' : 'var(--green)';
}
