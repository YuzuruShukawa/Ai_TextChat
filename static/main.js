let currentSessionId = null;
let sessions = [];
let isLoading = false;
let renamePollingTimer = null;
let aiName = "AI助手";
let aiAvatar = "/static/ai_avatar.png";
let personas = [];
let personaAvatarTemp = "/static/ai_avatar.png";
let currentDetailPersonaId = null;

document.addEventListener('DOMContentLoaded', () => {
    loadSessions();
    bindUI();
    collapseSidebar(false);
    document.getElementById('showSidebarBtn').onclick = function() {
        collapseSidebar(false);
    };
});

function bindUI() {
    document.getElementById('sendBtn').onclick = sendMessage;
    document.getElementById('messageInput').onkeypress = (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault(); sendMessage();
        }
    };
    document.getElementById('newSessionBtn').onclick = newSession;
    document.getElementById('terminateBtn').onclick = terminateSession;
    document.getElementById('openSettingsBtn').onclick = () => {
        loadPersonas();
        document.getElementById('settingsPanel').classList.remove('hidden');
    };
    document.getElementById('closeSettingsBtn').onclick = () => {
        document.getElementById('settingsPanel').classList.add('hidden');
    };
    document.getElementById('renameModalSaveBtn').onclick = renameSessionConfirm;
    document.getElementById('renameModalCancelBtn').onclick = () => {
        document.getElementById('renameModal').classList.add('hidden');
    };
    document.getElementById('toggleSidebarBtn').onclick = function () {
        let sidebar = document.getElementById('sidebar');
        let collapsed = sidebar.classList.contains('sidebar-collapsed');
        collapseSidebar(!collapsed);
    };
    document.getElementById('addPersonaBtn').onclick = showAddPersonaModal;
    document.getElementById('closePersonaModalBtn').onclick = closePersonaModal;
    document.getElementById('savePersonaBtn').onclick = savePersona;
    document.getElementById('personaAvatarInput').onchange = async function () {
        const fileInput = this;
        if (fileInput.files && fileInput.files[0]) {
            const fd = new FormData();
            fd.append('avatar', fileInput.files[0]);
            let res = await fetch('/api/upload_avatar', { method: 'POST', body: fd });
            let data = await res.json();
            if (data.url) {
                personaAvatarTemp = data.url;
                document.getElementById('personaAvatarPreview').src = personaAvatarTemp;
            }
        }
    };
    document.getElementById('closePersonaDetailBtn').onclick = function () {
        document.getElementById('personaDetailModal').classList.add('hidden');
    };
    document.getElementById('editPersonaBtn').onclick = function() {
        document.getElementById('personaDetailModal').classList.add('hidden');
        showEditPersonaModal(currentDetailPersonaId);
    };
}

function collapseSidebar(collapsed) {
    let sidebar = document.getElementById('sidebar');
    let btn = document.getElementById('toggleSidebarBtn');
    let showSidebarBtn = document.getElementById('showSidebarBtn');
    if (collapsed) {
        sidebar.classList.add('sidebar-collapsed');
        btn.classList.add('collapsed');
        btn.style.display = 'none';
        showSidebarBtn.style.display = '';
        showSidebarBtn.classList.remove('hidden');
        showSidebarBtn.style.zIndex = 2000;
        setTimeout(()=>document.getElementById('messageInput').focus(), 300);
    } else {
        sidebar.classList.remove('sidebar-collapsed');
        btn.classList.remove('collapsed');
        btn.style.display = '';
        showSidebarBtn.style.display = 'none';
        showSidebarBtn.classList.add('hidden');
        setTimeout(()=>document.getElementById('messageInput').focus(), 300);
    }
}

async function loadSessions() {
    let res = await fetch('/api/sessions');
    sessions = await res.json();
    renderSessionList();
    if (sessions.length > 0) {
        if (!currentSessionId || !sessions.find(s => s.id === currentSessionId)) {
            switchSession(sessions[0].id);
        }
    } else {
        newSession();
    }
}
async function newSession() {
    let res = await fetch('/api/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ modelName: "Deepseek", personality: "", aiName: "AI助手", aiAvatar: "/static/ai_avatar.png" })
    });
    let data = await res.json();
    await loadSessions();
    switchSession(data.sessionId);
}
function renderSessionList() {
    const ul = document.getElementById('sessionList');
    ul.innerHTML = '';
    sessions.forEach(sess => {
        const li = document.createElement('li');
        li.className = (sess.id === currentSessionId ? 'bg-blue-100 border-blue-300 text-blue-900' : 'text-blue-700 hover:bg-blue-50 hover:text-blue-900') +
            ' px-4 py-2 mb-2 rounded-lg cursor-pointer flex justify-between items-center group border transition';
        li.onclick = () => switchSession(sess.id);
        li.innerHTML = `
            <span class="truncate max-w-[110px]">${sess.name}</span>
            <span class="flex gap-1 ml-2 opacity-0 group-hover:opacity-100 transition">
                <button class="renameSessBtn text-blue-400 hover:text-blue-700 rounded-full p-1" title="重命名" onclick="event.stopPropagation();openRenameModal('${sess.id}')">✏️</button>
                <button class="deleteSessBtn text-pink-400 hover:text-pink-700 rounded-full p-1" title="删除" onclick="event.stopPropagation();deleteSession('${sess.id}')">🗑️</button>
            </span>`;
        ul.appendChild(li);
    });
}
async function switchSession(sid) {
    currentSessionId = sid;
    let sess = sessions.find(s=>s.id===sid);
    document.getElementById('currentSessionName').textContent = sess ? sess.name : '';
    document.getElementById('mainAiAvatar').src = sess ? (sess.ai_avatar || '/static/ai_avatar.png') : '/static/ai_avatar.png';
    aiName = sess ? (sess.ai_name || 'AI助手') : 'AI助手';
    aiAvatar = sess ? (sess.ai_avatar || '/static/ai_avatar.png') : '/static/ai_avatar.png';
    renderSessionList();
    await renderMessages();
}
async function renderMessages() {
    const div = document.getElementById('chatMessages');
    div.innerHTML = '';
    if (!currentSessionId) return;
    let res = await fetch(`/api/messages?sessionId=${currentSessionId}`);
    let msgs = await res.json();
    let sess = sessions.find(s => s.id === currentSessionId);
    let terminated = sess?.terminated == 1 || sess?.terminated === true;
    msgs.forEach(m => {
        addMessageBubble(m.role, m.content, m.meta, sess?.ai_name, sess?.ai_avatar);
    });
    if (terminated) {
        const endDiv = document.createElement('div');
        endDiv.className = 'bg-pink-100 border border-pink-300 text-pink-700 p-4 rounded-xl text-center font-bold';
        endDiv.textContent = '本次会话已结束，感谢您的使用';
        div.appendChild(endDiv);
        document.getElementById('messageInput').disabled = true;
        document.getElementById('sendBtn').disabled = true;
        document.getElementById('terminateBtn').disabled = true;
    } else {
        document.getElementById('messageInput').disabled = false;
        document.getElementById('sendBtn').disabled = false;
        document.getElementById('terminateBtn').disabled = false;
    }
}
function addMessageBubble(role, content, meta, aiNameParam, aiAvatarParam) {
    const div = document.createElement('div');
    if (role === 'assistant') {
        div.innerHTML = `
        <div class="flex items-start gap-3">
          <img src="${aiAvatarParam || aiAvatar}" class="w-10 h-10 rounded-full border border-blue-200 bg-white object-cover" />
          <div>
            <div class="font-bold text-blue-700 mb-1">${aiNameParam || aiName}</div>
            <div class="p-4 rounded-xl shadow-lg max-w-2xl glass text-blue-900 border border-blue-100 animate-fade-in">${marked.parse(content)}</div>
            ${meta ? `<div class="text-xs text-blue-400 mt-2">${meta}</div>` : ''}
          </div>
        </div>`;
    } else {
        div.className = 'p-4 rounded-xl shadow-lg max-w-2xl bg-blue-100 text-blue-900 ml-auto border border-blue-200 animate-fade-in';
        div.innerHTML = escapeHtml(content);
        if (meta) {
            const metaDiv = document.createElement('div');
            metaDiv.className = 'text-xs text-blue-400 mt-2';
            metaDiv.textContent = meta;
            div.appendChild(metaDiv);
        }
    }
    document.getElementById('chatMessages').appendChild(div);
    document.querySelectorAll('pre code').forEach(el => hljs.highlightElement(el));
    scrollToLatest();
}
function scrollToLatest() {
    const div = document.getElementById('chatMessages');
    div.scrollTop = div.scrollHeight;
}
async function sendMessage() {
    if (isLoading || !currentSessionId) return;
    const input = document.getElementById('messageInput');
    const message = input.value.trim();
    if (!message) return;
    input.value = '';
    isLoading = true;
    addMessageBubble('user', message, null);
    addMessageBubble('assistant', '正在思考中...', null, aiName, aiAvatar);
    scrollToLatest();
    try {
        const res = await fetch('/api/chat', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ sessionId: currentSessionId, message })
        });
        const data = await res.json();
        removeLoadingBubble();
        if (data.terminated) {
            addMessageBubble('assistant', data.summary, '对话总结');
            const div = document.createElement('div');
            div.className = 'bg-pink-100 border border-pink-300 text-pink-700 p-4 rounded-xl text-center font-bold';
            div.textContent = data.endMessage || '本次会话已结束，感谢您的使用';
            document.getElementById('chatMessages').appendChild(div);
            document.getElementById('messageInput').disabled = true;
            document.getElementById('sendBtn').disabled = true;
            document.getElementById('terminateBtn').disabled = true;
            await loadSessions();
            let sess = sessions.find(s => s.id === currentSessionId);
            document.getElementById('currentSessionName').textContent = data.newTitle || (sess ? sess.name : '');
        } else if (res.ok || data.message) {
            addMessageBubble('assistant', data.message, '响应时间: ' + data.elapsedTime, data.aiName, data.aiAvatar);
            let sess = sessions.find(s => s.id === currentSessionId);
            if (sess && sess.name === "新对话" && !renamePollingTimer) {
                startRenamePolling();
            }
        } else {
            showError('发送失败: ' + data.message);
        }
    } catch (err) {
        removeLoadingBubble();
        showError('网络错误: ' + err.message);
    } finally {
        isLoading = false;
    }
}
function startRenamePolling() {
    let pollingCount = 0;
    renamePollingTimer = setInterval(async () => {
        pollingCount++;
        await loadSessions();
        let sess = sessions.find(s => s.id === currentSessionId);
        if (sess && sess.name !== "新对话") {
            clearInterval(renamePollingTimer);
            renamePollingTimer = null;
            renderSessionList();
        }
        if (pollingCount > 10) {
            clearInterval(renamePollingTimer);
            renamePollingTimer = null;
        }
    }, 3000);
}
async function terminateSession() {
    if (!currentSessionId) return;
    if (!confirm('确定要终止本次会话吗？')) return;
    let res = await fetch('/api/session/terminate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sessionId: currentSessionId })
    });
    let data = await res.json();
    if (data.result === 'success') {
        await loadSessions();
        await renderMessages();
        document.getElementById('currentSessionName').textContent = data.newTitle || '';
    } else {
        showError('终止失败！');
    }
}
function removeLoadingBubble() {
    const bubbles = document.querySelectorAll('#chatMessages > div');
    if (bubbles.length) {
        const last = bubbles[bubbles.length - 1];
        if (last.textContent.includes("正在思考")) last.remove();
    }
}
function showError(msg) {
    const div = document.createElement('div');
    div.className = 'bg-pink-200 text-pink-800 p-4 rounded-xl mt-2 shadow-xl animate-shake border border-pink-300';
    div.textContent = msg;
    document.getElementById('chatMessages').appendChild(div);
    scrollToLatest();
}
async function deleteSession(sessId) {
    if (!confirm('确定要删除该历史会话及所有消息吗？此操作不可恢复！')) return;
    let res = await fetch('/api/session/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sessionId: sessId })
    });
    let data = await res.json();
    if (data.result === 'success') {
        await loadSessions();
        if (sessions.length > 0) {
            switchSession(sessions[0].id);
        } else {
            currentSessionId = null;
            document.getElementById('chatMessages').innerHTML = '';
            document.getElementById('currentSessionName').textContent = '';
        }
    } else {
        showError('删除失败！');
    }
}
function openRenameModal(sessId) {
    const sess = sessions.find(s=>s.id===sessId);
    document.getElementById('renameModal').classList.remove('hidden');
    document.getElementById('renameModalInput').value = sess ? sess.name : '';
    document.getElementById('renameModal').dataset.sessId = sessId;
    document.getElementById('renameModalInput').focus();
}
async function renameSessionConfirm() {
    const sessId = document.getElementById('renameModal').dataset.sessId;
    const newName = document.getElementById('renameModalInput').value.trim();
    if (!newName) return showError('名称不能为空');
    let res = await fetch('/api/session/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sessionId: sessId, newName })
    });
    let data = await res.json();
    if (data.result === 'success') {
        document.getElementById('renameModal').classList.add('hidden');
        await loadSessions();
        switchSession(sessId);
    } else {
        showError('重命名失败！');
    }
}
function escapeHtml(s) {
    return s.replace(/[&<>"']/g, function(m) {
        return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m];
    });
}

// ------------ 人格管理 -------------
async function loadPersonas() {
    let res = await fetch('/api/personas');
    personas = await res.json();
    renderPersonaList();
}
function renderPersonaList() {
    const box = document.getElementById('personaList');
    box.innerHTML = '';
    personas.forEach(p => {
        let card = document.createElement('div');
        card.className = 'glass shadow border flex gap-3 p-3 rounded-lg items-center cursor-pointer relative group';
        card.onclick = () => showPersonaDetail(p.id);
        card.innerHTML = `
            <img src="${p.avatar||'/static/ai_avatar.png'}" class="w-14 h-14 rounded-full border border-blue-200 object-cover" />
            <div class="flex-1 min-w-0">
              <div class="font-bold text-blue-800 persona-card-row">${escapeHtml(p.name)}</div>
              <div class="text-xs text-blue-500 persona-card-row">${escapeHtml(p.identity||'')}</div>
              <div class="text-xs text-blue-400 persona-card-row multiline">${escapeHtml(p.personality||'')}</div>
            </div>
            <button class="absolute top-2 right-2 text-pink-400 hover:text-pink-700 text-lg hidden group-hover:block" onclick="event.stopPropagation();deletePersonaCard(${p.id})">🗑️</button>
            <button class="absolute top-2 right-10 text-blue-400 hover:text-blue-700 text-lg hidden group-hover:block" onclick="event.stopPropagation();usePersonaForCurrentSession(${p.id})">切换</button>
            <button class="absolute bottom-2 right-2 bg-blue-100 text-blue-700 text-xs rounded px-2 py-1 hover:bg-blue-200 transition hidden group-hover:block" onclick="event.stopPropagation();showPersonaDetail(${p.id})">详情</button>
        `;
        box.appendChild(card);
    });
}
async function showPersonaDetail(personaId) {
    currentDetailPersonaId = personaId;
    let p = personas.find(x=>x.id===personaId);
    if (!p) {
        let res = await fetch('/api/persona/' + personaId);
        p = await res.json();
    }
    document.getElementById('personaDetailContent').innerHTML = `
        <div class="flex flex-col items-center mb-3">
          <img src="${p.avatar||'/static/ai_avatar.png'}" class="w-20 h-20 rounded-full border border-blue-200 object-cover mb-2" />
          <div class="font-bold text-blue-800 text-lg">${escapeHtml(p.name)}</div>
        </div>
        <div class="mb-2"><span class="font-bold text-blue-700">身份：</span>${escapeHtml(p.identity||'')}</div>
        <div class="mb-2"><span class="font-bold text-blue-700">外貌：</span>${escapeHtml(p.appearance||'')}</div>
        <div class="mb-2"><span class="font-bold text-blue-700">性格：</span>${escapeHtml(p.personality||'')}</div>
    `;
    document.getElementById('personaDetailModal').classList.remove('hidden');
}
function closePersonaModal() {
    document.getElementById('personaModal').classList.add('hidden');
}
function showAddPersonaModal() {
    personaAvatarTemp = "/static/ai_avatar.png";
    document.getElementById('personaModalTitle').textContent = '新增人格';
    document.getElementById('personaIdInput').value = '';
    document.getElementById('personaAvatarPreview').src = personaAvatarTemp;
    document.getElementById('personaNameInput').value = '';
    document.getElementById('personaIdentityInput').value = '';
    document.getElementById('personaAppearanceInput').value = '';
    document.getElementById('personaPersonalityInput').value = '';
    document.getElementById('personaModal').classList.remove('hidden');
}
async function showEditPersonaModal(id) {
    let p = personas.find(x=>x.id===id);
    if (!p) return;
    personaAvatarTemp = p.avatar||"/static/ai_avatar.png";
    document.getElementById('personaModalTitle').textContent = '编辑人格';
    document.getElementById('personaIdInput').value = p.id;
    document.getElementById('personaAvatarPreview').src = personaAvatarTemp;
    document.getElementById('personaNameInput').value = p.name;
    document.getElementById('personaIdentityInput').value = p.identity||'';
    document.getElementById('personaAppearanceInput').value = p.appearance||'';
    document.getElementById('personaPersonalityInput').value = p.personality||'';
    document.getElementById('personaModal').classList.remove('hidden');
}
async function savePersona() {
    let id = document.getElementById('personaIdInput').value;
    let data = {
        id: id ? Number(id) : undefined,
        name: document.getElementById('personaNameInput').value.trim(),
        avatar: personaAvatarTemp,
        identity: document.getElementById('personaIdentityInput').value.trim(),
        appearance: document.getElementById('personaAppearanceInput').value.trim(),
        personality: document.getElementById('personaPersonalityInput').value.trim()
    };
    if (!data.name) return showError('名称不能为空');
    let res = await fetch('/api/persona', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    });
    let resp = await res.json();
    if (resp.result === 'success') {
        await loadPersonas();
        closePersonaModal();
    } else {
        showError('保存失败');
    }
}
async function deletePersonaCard(id) {
    if (!confirm('确定要删除该人格吗？')) return;
    let res = await fetch('/api/persona/' + id, { method: 'DELETE' });
    let data = await res.json();
    if (data.result === 'success') {
        await loadPersonas();
    } else {
        showError('删除失败');
    }
}
async function usePersonaForCurrentSession(personaId) {
    if (!currentSessionId) return showError('请先选择会话');
    let res = await fetch('/api/session/use_persona', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sessionId: currentSessionId, personaId })
    });
    let data = await res.json();
    if (data.result === 'success') {
        await loadSessions();
        await renderMessages();
        document.getElementById('settingsPanel').classList.add('hidden');
    } else {
        showError('切换人格失败');
    }
}