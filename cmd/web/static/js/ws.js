let ws;

function connect() {
  ws = new WebSocket('ws://' + location.host + '/ui-ws');
  ws.onopen = () => addInfo('Connected to server');
  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data);

    if (msg.type === 'info' && msg.data && msg.data.event) {
      const ev = msg.data.event;
      if (ev.includes('disconnected')) {
        setStatus(false, 'Client disconnected');
        addInfo('go-mud disconnected');
        stopLoadingSound();
      } else {
        setStatus(true, 'Client connected');
        addInfo('go-mud connected');
      }
      return;
    }

    if (msg.type === 'status' && msg.data) {
      const s = msg.data.state;
      if (s === 'connected') setStatus(true, 'Connected');
      else if (s === 'executing') { setStatus(true, 'Executing...'); updateSentCard(msg.id, 'executing'); }
      else if (s === 'queued') updateSentCard(msg.id, 'queued');
      else if (s === 'idle') setStatus(true, 'Idle');
      return;
    }

    if (msg.type === 'progress' && msg.data) {
      updateProgress(msg.id, msg.data.current, msg.data.total, msg.data.command);
      return;
    }

    if (msg.type === 'result' && msg.data && msg.data.results) {
      const totalMs = msg.data.results.reduce((s, r) => s + (r.duration_ms || 0), 0);
      const hasErr = msg.data.results.some(hasOutputError);
      resolveSentCard(msg.id, totalMs, hasErr);
      addResultsToGroup(msg.id, msg.data.results);
      return;
    }

    if (msg.type === 'error') {
      if (msg.id) resolveSentCard(msg.id, 0, true);
      addError((msg.data && msg.data.message) || JSON.stringify(msg));
      return;
    }

    if (msg.type === 'pong') return;
  };
  ws.onclose = () => {
    setStatus(false, 'Server down');
    addError('Connection lost, reconnecting...');
    stopLoadingSound();
    setTimeout(connect, 2000);
  };
}

function sendCmd() {
  const raw = cmdInput.value.trim();
  if (!raw) return;
  cmdInput.value = '';

  fetch('/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: 'commands=' + encodeURIComponent(raw),
  }).then(r => {
    if (!r.ok) return r.text().then(t => { throw new Error(t); });
    return r.json();
  }).then(data => {
    addSentCard(data.id, raw);
  }).catch(err => {
    addError('Send failed: ' + err.message);
  });
}

function clearLog() {
  logContainer.innerHTML = '';
  stopLoadingSound();
}

cmdInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') sendCmd();
});

connect();
cmdInput.focus();
