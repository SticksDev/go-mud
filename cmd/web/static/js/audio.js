let muted = false;
const sndProcessing = new Audio('/static/audio/Common_Processing_Loop.wav');
const sndSubmit = new Audio('/static/audio/Common_Submit_A.wav');
const sndSuccess = new Audio('/static/audio/Common_Success_A.wav');
const sndError = new Audio('/static/audio/Common_Error_A.wav');
sndProcessing.loop = true;
sndProcessing.volume = 0.4;
sndSubmit.volume = 0.3;
sndSuccess.volume = 0.3;
sndError.volume = 0.3;

function startLoadingSound() {
  stopLoadingSound();
  if (muted) return;
  sndProcessing.currentTime = 0;
  sndProcessing.play().catch(() => {});
}

function stopLoadingSound() {
  sndProcessing.pause();
  sndProcessing.currentTime = 0;
}

function playSubmit() {
  if (muted) return;
  sndSubmit.currentTime = 0;
  sndSubmit.play().catch(() => {});
}

function playSuccess() {
  if (muted) return;
  sndSuccess.currentTime = 0;
  sndSuccess.play().catch(() => {});
}

function playError() {
  if (muted) return;
  sndError.currentTime = 0;
  sndError.play().catch(() => {});
}

function toggleMute() {
  muted = !muted;
  document.getElementById('muteBtn').classList.toggle('muted', muted);
  document.getElementById('muteIcon').innerHTML = muted
    ? '<line x1="1" y1="1" x2="23" y2="23"></line><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"></polygon>'
    : '<polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"></polygon><path d="M19.07 4.93a10 10 0 0 1 0 14.14"></path><path d="M15.54 8.46a5 5 0 0 1 0 7.08"></path>';
  if (muted) stopLoadingSound();
}
