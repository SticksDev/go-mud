const logContainer = document.getElementById('logContainer');
const indicator = document.getElementById('indicator');
const statusText = document.getElementById('statusText');
const cmdInput = document.getElementById('cmd');

const svgCheck = '<svg class="check-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 8.5 L6.5 12 L13 4"/></svg>';
const svgError = '<svg class="error-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 4 L12 12 M12 4 L4 12"/></svg>';
const svgUserSwitch = '<svg class="user-switch-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>';

function setStatus(connected, label) {
  indicator.className = 'indicator' + (connected ? ' on' : '');
  statusText.textContent = label;
}

function clearEmpty() {
  const es = document.getElementById('emptyState');
  if (es) es.remove();
}

function scrollToBottom() {
  logContainer.scrollTop = logContainer.scrollHeight;
}

function fmtDuration(ms) {
  if (!ms) return '';
  if (ms < 1000) return ms + 'ms';
  return (ms / 1000).toFixed(1) + 's';
}

function renderColorTags(raw) {
  const escaped = raw.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  let html = escaped.replace(/&lt;color=(#[0-9A-Fa-f]{6})[0-9A-Fa-f]{0,2}&gt;/g, '<span style="color:$1">');
  html = html.replace(/&lt;\/color&gt;/g, '</span>');
  return html;
}

function parseCommands(text) {
  try {
    const parsed = JSON.parse(text);
    if (Array.isArray(parsed)) return parsed;
  } catch {}
  return text.split(',').map(s => s.trim()).filter(Boolean);
}

function hasOutputError(r) {
  return r.error || r.timed_out || (r.output || '').includes('ERROR');
}

function detectUserSwitch(text) {
  const match = text.match(/Active user switched to (\S+)/);
  return match ? match[1] : null;
}
