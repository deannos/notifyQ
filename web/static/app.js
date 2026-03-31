'use strict';

// ---- State ----
const state = {
  token: localStorage.getItem('nq_token') || '',
  user: JSON.parse(localStorage.getItem('nq_user') || 'null'),
  ws: null,
  notifOffset: 0,
  notifLimit: 20,
  notifTotal: 0,
  currentPanel: 'notifications',
};

const api = {
  async req(method, path, body) {
    const opts = {
      method,
      headers: { 'Content-Type': 'application/json' },
    };
    if (state.token) opts.headers['Authorization'] = 'Bearer ' + state.token;
    if (body) opts.body = JSON.stringify(body);
    const res = await fetch(path, opts);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  },
  get: (path) => api.req('GET', path),
  post: (path, body) => api.req('POST', path, body),
  put: (path, body) => api.req('PUT', path, body),
  del: (path) => api.req('DELETE', path),
};

// ---- Screen management ----
function showScreen(id) {
  document.querySelectorAll('.screen').forEach(el => el.classList.add('hidden'));
  document.getElementById(id).classList.remove('hidden');
}

function showPanel(name) {
  document.querySelectorAll('.panel').forEach(el => el.classList.add('hidden'));
  document.getElementById('panel-' + name).classList.remove('hidden');
  document.querySelectorAll('.nav-item').forEach(el => {
    el.classList.toggle('active', el.dataset.panel === name);
  });
  state.currentPanel = name;
  if (name === 'notifications') loadNotifications();
  if (name === 'apps') loadApps();
  if (name === 'users') loadUsers();
}

// ---- Auth ----
document.getElementById('go-register').addEventListener('click', (e) => {
  e.preventDefault(); showScreen('register-screen');
});
document.getElementById('go-login').addEventListener('click', (e) => {
  e.preventDefault(); showScreen('login-screen');
});

document.getElementById('login-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const errEl = document.getElementById('login-error');
  errEl.textContent = '';
  try {
    const data = await api.post('/auth/login', {
      username: document.getElementById('login-username').value,
      password: document.getElementById('login-password').value,
    });
    state.token = data.token;
    state.user = data.user;
    localStorage.setItem('nq_token', state.token);
    localStorage.setItem('nq_user', JSON.stringify(state.user));
    bootDashboard();
  } catch (err) {
    errEl.textContent = err.message;
  }
});

document.getElementById('register-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const errEl = document.getElementById('register-error');
  errEl.textContent = '';
  try {
    await api.post('/auth/register', {
      username: document.getElementById('reg-username').value,
      password: document.getElementById('reg-password').value,
    });
    showScreen('login-screen');
  } catch (err) {
    errEl.textContent = err.message;
  }
});

document.getElementById('logout-btn').addEventListener('click', () => {
  state.token = '';
  state.user = null;
  localStorage.removeItem('nq_token');
  localStorage.removeItem('nq_user');
  if (state.ws) { state.ws.close(); state.ws = null; }
  showScreen('login-screen');
});

// ---- Dashboard Boot ----
function bootDashboard() {
  document.getElementById('user-badge').textContent = '👤 ' + (state.user?.username || '');
  if (state.user?.is_admin) {
    document.querySelectorAll('.admin-only').forEach(el => el.classList.remove('hidden'));
  }
  showScreen('dashboard');
  showPanel('notifications');
  connectWS();
}

// ---- Navigation ----
document.querySelectorAll('.nav-item').forEach(btn => {
  btn.addEventListener('click', () => showPanel(btn.dataset.panel));
});

// ---- WebSocket ----
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const url = `${proto}://${location.host}/ws?token=${state.token}`;
  const ws = new WebSocket(url);
  state.ws = ws;
  const dot = document.getElementById('ws-status');

  ws.onopen = () => {
    dot.className = 'ws-dot connected';
    dot.title = 'WebSocket connected';
  };
  ws.onclose = () => {
    dot.className = 'ws-dot disconnected';
    dot.title = 'WebSocket disconnected';
    // Reconnect after 5s if still logged in.
    if (state.token) setTimeout(connectWS, 5000);
  };
  ws.onerror = () => ws.close();
  ws.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data);
      if (msg.event === 'notification') {
        handleIncomingNotification(msg.notification);
      }
    } catch { /* ignore malformed */ }
  };
}

function handleIncomingNotification(notif) {
  // Refresh notification panel if visible.
  if (state.currentPanel === 'notifications') {
    prependNotification(notif);
    state.notifTotal++;
    updateStats();
  }
  // Browser notification (if permitted).
  if (Notification.permission === 'granted') {
    new Notification(notif.title, { body: notif.message });
  }
}

// ---- Notifications ----
async function loadNotifications() {
  try {
    const data = await api.get(`/api/v1/notification?limit=${state.notifLimit}&offset=${state.notifOffset}`);
    state.notifTotal = data.total || 0;
    renderNotifications(data.notifications || []);
    updateStats();
    updatePagination();
  } catch (err) {
    console.error('loadNotifications', err);
  }
}

function renderNotifications(notifs) {
  const list = document.getElementById('notif-list');
  if (!notifs.length) {
    list.innerHTML = '<div class="empty-state">No notifications yet.</div>';
    return;
  }
  list.innerHTML = notifs.map(n => notifCard(n)).join('');
  list.querySelectorAll('.mark-read-btn').forEach(btn => {
    btn.addEventListener('click', () => markRead(btn.dataset.id));
  });
  list.querySelectorAll('.delete-notif-btn').forEach(btn => {
    btn.addEventListener('click', () => deleteNotification(btn.dataset.id, btn.closest('.notif-card')));
  });
}

function prependNotification(notif) {
  const list = document.getElementById('notif-list');
  const empty = list.querySelector('.empty-state');
  if (empty) empty.remove();
  const div = document.createElement('div');
  div.innerHTML = notifCard(notif);
  const card = div.firstElementChild;
  card.querySelector('.mark-read-btn')?.addEventListener('click', () => markRead(notif.id));
  card.querySelector('.delete-notif-btn')?.addEventListener('click', () => deleteNotification(notif.id, card));
  list.prepend(card);
}

function notifCard(n) {
  const priority = n.priority >= 8 ? 'high' : n.priority >= 4 ? 'mid' : 'low';
  const appName = n.app?.name || n.app_id;
  const date = new Date(n.created_at).toLocaleString();
  return `
    <div class="notif-card ${n.read ? '' : 'unread'}" data-id="${n.id}">
      <div class="notif-priority priority-${priority}">${n.priority}</div>
      <div class="notif-body">
        <div class="notif-title">${esc(n.title)}</div>
        <div class="notif-message">${esc(n.message)}</div>
        <div class="notif-meta">
          <span class="notif-app-tag">${esc(appName)}</span>
          <span>${date}</span>
          ${n.read ? '<span>&#10003; read</span>' : ''}
        </div>
      </div>
      <div class="notif-actions">
        ${!n.read ? `<button class="mark-read-btn" data-id="${n.id}" title="Mark read">&#10003;</button>` : ''}
        <button class="delete-notif-btn" data-id="${n.id}" title="Delete">&#128465;</button>
      </div>
    </div>`;
}

async function markRead(id) {
  try {
    await api.put(`/api/v1/notification/${id}/read`);
    const card = document.querySelector(`.notif-card[data-id="${id}"]`);
    if (card) {
      card.classList.remove('unread');
      card.querySelector('.mark-read-btn')?.remove();
    }
    updateStats();
  } catch { /* ignore */ }
}

async function deleteNotification(id, cardEl) {
  try {
    await api.del(`/api/v1/notification/${id}`);
    cardEl?.remove();
    state.notifTotal = Math.max(0, state.notifTotal - 1);
    updateStats();
    updatePagination();
    if (!document.querySelector('.notif-card')) {
      document.getElementById('notif-list').innerHTML = '<div class="empty-state">No notifications yet.</div>';
    }
  } catch { /* ignore */ }
}

document.getElementById('mark-all-read').addEventListener('click', async () => {
  // Mark each visible one read.
  document.querySelectorAll('.notif-card.unread').forEach(c => markRead(c.dataset.id));
});

document.getElementById('delete-all-notif').addEventListener('click', async () => {
  if (!confirm('Delete all notifications?')) return;
  await api.del('/api/v1/notification');
  state.notifOffset = 0;
  loadNotifications();
});

// Pagination
document.getElementById('notif-prev').addEventListener('click', () => {
  state.notifOffset = Math.max(0, state.notifOffset - state.notifLimit);
  loadNotifications();
});
document.getElementById('notif-next').addEventListener('click', () => {
  state.notifOffset += state.notifLimit;
  loadNotifications();
});

function updatePagination() {
  const pages = Math.ceil(state.notifTotal / state.notifLimit);
  const current = Math.floor(state.notifOffset / state.notifLimit) + 1;
  const pagEl = document.getElementById('notif-pagination');
  const prev = document.getElementById('notif-prev');
  const next = document.getElementById('notif-next');
  const info = document.getElementById('notif-page-info');
  if (pages <= 1) { pagEl.classList.add('hidden'); return; }
  pagEl.classList.remove('hidden');
  prev.disabled = current <= 1;
  next.disabled = current >= pages;
  info.textContent = `Page ${current} of ${pages}`;
}

function updateStats() {
  const total = document.querySelectorAll('.notif-card').length;
  const unread = document.querySelectorAll('.notif-card.unread').length;
  document.getElementById('stat-total').textContent = state.notifTotal || total;
  document.getElementById('stat-unread').textContent = unread;
}

// ---- Apps ----
async function loadApps() {
  try {
    const apps = await api.get('/api/v1/application');
    renderApps(apps || []);
  } catch (err) {
    console.error('loadApps', err);
  }
}

function renderApps(apps) {
  const list = document.getElementById('app-list');
  if (!apps.length) {
    list.innerHTML = '<div class="empty-state">No applications yet. Create one to start sending notifications.</div>';
    return;
  }
  list.innerHTML = apps.map(a => `
    <div class="app-card" data-id="${a.id}">
      <div class="app-icon">&#128273;</div>
      <div class="app-info">
        <div class="app-name">${esc(a.name)}</div>
        <div class="app-desc">${esc(a.description || '')}</div>
        <div class="app-id">ID: ${a.id}</div>
      </div>
      <div class="app-actions">
        <button class="btn btn-sm btn-outline rotate-token-btn" data-id="${a.id}">Rotate Token</button>
        <button class="btn btn-sm btn-danger delete-app-btn" data-id="${a.id}">Delete</button>
      </div>
    </div>`).join('');

  list.querySelectorAll('.delete-app-btn').forEach(btn => {
    btn.addEventListener('click', () => deleteApp(btn.dataset.id, btn.closest('.app-card')));
  });
  list.querySelectorAll('.rotate-token-btn').forEach(btn => {
    btn.addEventListener('click', () => rotateToken(btn.dataset.id));
  });
}

document.getElementById('add-app-btn').addEventListener('click', () => {
  document.getElementById('app-name').value = '';
  document.getElementById('app-desc').value = '';
  document.getElementById('app-token-result').classList.add('hidden');
  document.getElementById('add-app-error').textContent = '';
  document.getElementById('submit-app').disabled = false;
  document.getElementById('add-app-form').classList.remove('hidden');
});
document.getElementById('cancel-app').addEventListener('click', () => {
  document.getElementById('add-app-form').classList.add('hidden');
  loadApps();
});

document.getElementById('submit-app').addEventListener('click', async () => {
  const errEl = document.getElementById('add-app-error');
  errEl.textContent = '';
  const name = document.getElementById('app-name').value.trim();
  if (!name) { errEl.textContent = 'Name is required'; return; }
  try {
    const app = await api.post('/api/v1/application', {
      name,
      description: document.getElementById('app-desc').value,
    });
    document.getElementById('app-token-value').textContent = app.token;
    document.getElementById('app-token-result').classList.remove('hidden');
    document.getElementById('submit-app').disabled = true;
  } catch (err) {
    errEl.textContent = err.message;
  }
});

document.getElementById('copy-token').addEventListener('click', () => {
  navigator.clipboard.writeText(document.getElementById('app-token-value').textContent);
});

async function deleteApp(id, cardEl) {
  if (!confirm('Delete this application and all its notifications?')) return;
  try {
    await api.del('/api/v1/application/' + id);
    cardEl?.remove();
  } catch (err) {
    alert(err.message);
  }
}

async function rotateToken(id) {
  if (!confirm('Rotate the token? The old token will stop working immediately.')) return;
  try {
    const data = await api.post(`/api/v1/application/${id}/token`);
    alert('New token (save this — it won\'t be shown again):\n\n' + data.token);
  } catch (err) {
    alert(err.message);
  }
}

// ---- Users (admin) ----
async function loadUsers() {
  try {
    const users = await api.get('/api/v1/user');
    renderUsers(users || []);
  } catch (err) {
    console.error('loadUsers', err);
  }
}

function renderUsers(users) {
  const list = document.getElementById('user-list');
  list.innerHTML = users.map(u => `
    <div class="app-card">
      <div class="app-icon">&#128100;</div>
      <div class="app-info">
        <div class="app-name">${esc(u.username)} ${u.is_admin ? '&#9733;' : ''}</div>
        <div class="app-desc">Created: ${new Date(u.created_at).toLocaleDateString()}</div>
      </div>
      <div class="app-actions">
        <button class="btn btn-sm btn-danger delete-user-btn" data-id="${u.id}">Delete</button>
      </div>
    </div>`).join('') || '<div class="empty-state">No users.</div>';

  list.querySelectorAll('.delete-user-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (!confirm('Delete this user and all their data?')) return;
      try {
        await api.del('/api/v1/user/' + btn.dataset.id);
        btn.closest('.app-card').remove();
      } catch (err) { alert(err.message); }
    });
  });
}

document.getElementById('add-user-btn').addEventListener('click', () => {
  document.getElementById('add-user-error').textContent = '';
  document.getElementById('add-user-form').classList.remove('hidden');
});
document.getElementById('cancel-user').addEventListener('click', () => {
  document.getElementById('add-user-form').classList.add('hidden');
});
document.getElementById('submit-user').addEventListener('click', async () => {
  const errEl = document.getElementById('add-user-error');
  errEl.textContent = '';
  try {
    await api.post('/api/v1/user', {
      username: document.getElementById('new-user-name').value,
      password: document.getElementById('new-user-pass').value,
      is_admin: document.getElementById('new-user-admin').checked,
    });
    document.getElementById('add-user-form').classList.add('hidden');
    loadUsers();
  } catch (err) {
    errEl.textContent = err.message;
  }
});

// ---- Helpers ----
function esc(str) {
  return String(str ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// Request browser notification permission
if (Notification && Notification.permission === 'default') {
  Notification.requestPermission();
}

// ---- Boot ----
if (state.token && state.user) {
  bootDashboard();
} else {
  showScreen('login-screen');
}
