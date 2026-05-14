function buildResultSection(r) {
  const section = document.createElement('div');
  section.className = 'result-section';

  const switchedUser = detectUserSwitch(r.output || '');
  if (switchedUser) {
    const banner = document.createElement('div');
    banner.className = 'user-switch';
    banner.innerHTML = svgUserSwitch + '<span class="user-switch-text">Switched to ' + switchedUser + '</span>';
    section.appendChild(banner);
  }

  const header = document.createElement('div');
  header.className = 'result-header';

  const cmdEl = document.createElement('span');
  cmdEl.className = 'result-cmd';
  cmdEl.textContent = r.command;
  header.appendChild(cmdEl);

  if (r.timed_out) {
    const badge = document.createElement('span');
    badge.className = 'result-badge badge-timeout';
    badge.textContent = 'timeout';
    header.appendChild(badge);
  }

  if (r.error) {
    const badge = document.createElement('span');
    badge.className = 'result-badge badge-error';
    badge.textContent = 'error';
    header.appendChild(badge);
  }

  section.appendChild(header);

  const rawText = (r.raw_output || '').trim();
  const cleanText = (r.output || '').trim();
  const isJustSwitch = switchedUser && cleanText === 'Active user switched to ' + switchedUser;

  if (!isJustSwitch) {
    const output = document.createElement('div');
    if (rawText) {
      output.className = 'result-output';
      output.innerHTML = renderColorTags(rawText);
    } else if (cleanText) {
      output.className = 'result-output';
      output.textContent = cleanText;
    } else if (!r.error) {
      output.className = 'result-output empty-output';
      output.textContent = 'No output';
    }
    if (rawText || cleanText || !r.error) section.appendChild(output);
  }

  if (r.error) {
    const errEl = document.createElement('div');
    errEl.className = 'result-error';
    errEl.textContent = r.error;
    section.appendChild(errEl);
  }

  const jsonBlock = document.createElement('div');
  jsonBlock.className = 'json-block';
  jsonBlock.textContent = JSON.stringify(r, null, 2);

  const toggle = document.createElement('span');
  toggle.className = 'view-json';
  toggle.textContent = 'View JSON';
  toggle.onclick = () => {
    const open = jsonBlock.classList.toggle('open');
    toggle.textContent = open ? 'Hide JSON' : 'View JSON';
  };

  section.append(toggle, jsonBlock);
  return section;
}

function buildResultCard(results) {
  const hasError = results.some(hasOutputError);
  const card = document.createElement('div');
  card.className = 'card-result';
  if (hasError) card.classList.add('has-error');
  if (results.some(r => r.timed_out)) card.classList.add('timed-out');

  results.forEach((r, i) => {
    if (i > 0) {
      const divider = document.createElement('div');
      divider.className = 'result-divider';
      card.appendChild(divider);
    }
    card.appendChild(buildResultSection(r));
  });

  return card;
}

function addResultsToGroup(id, results) {
  const entry = pendingGroups[id];
  if (!entry) {
    clearEmpty();
    logContainer.appendChild(buildResultCard(results));
    scrollToBottom();
    return;
  }

  entry.resultsSlot.appendChild(buildResultCard(results));
  scrollToBottom();
  delete pendingGroups[id];
}

function addError(text) {
  clearEmpty();
  playError();
  const card = document.createElement('div');
  card.className = 'card-error';
  card.textContent = text;
  logContainer.appendChild(card);
  scrollToBottom();
}

function addInfo(text) {
  clearEmpty();
  const el = document.createElement('div');
  el.className = 'line-info';
  el.textContent = text;
  logContainer.appendChild(el);
  scrollToBottom();
}
