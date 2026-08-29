<template>
  <t-drawer v-model:visible="drawerVisible" placement="right" size="min(900px, 92vw)" :footer="false">
    <template #header>
      <div class="wb-title"><t-icon name="code" /> 沙箱工作台</div>
    </template>
    <t-tabs v-model="tab" class="wb-tabs">
      <t-tab-panel value="terminal" label="终端">
        <section ref="terminalBox" class="terminal" aria-live="polite">
          <div class="terminal-toolbar">
            <code>/workspace</code>
            <t-button v-if="running" size="small" theme="danger" variant="outline" @click="interrupt">中断</t-button>
            <t-tag v-else size="small" variant="light">{{ terminalStatus }}</t-tag>
          </div>
          <pre ref="outputBox" class="terminal-output"><span v-if="!output" class="terminal-muted">在下方输入命令。命令在当前会话的隔离沙箱中运行。</span>{{ output }}</pre>
          <form class="terminal-input" @submit.prevent="run">
            <span class="terminal-prompt">$</span>
            <input v-model="command" :disabled="running" autocomplete="off" spellcheck="false" placeholder="输入命令后按 Enter" />
            <t-button type="submit" theme="primary" :disabled="running || !command.trim()">运行</t-button>
          </form>
        </section>
      </t-tab-panel>
      <t-tab-panel value="files" label="文件">
        <section class="files">
          <div class="files-toolbar">
            <button class="path-up" :disabled="currentPath === outputRoot" @click="goUp"><t-icon name="chevron-left" /></button>
            <code>{{ currentPath }}</code>
            <span class="files-spacer" />
            <input ref="fileInput" type="file" hidden @change="onUpload" />
            <t-button size="small" variant="outline" @click="fileInput?.click()">上传</t-button>
            <t-button size="small" variant="text" :loading="loadingFiles" @click="loadFiles"><t-icon name="refresh" /></t-button>
          </div>
          <div v-if="loadingFiles" class="files-empty"><t-loading size="small" /></div>
          <div v-else-if="!entries.length" class="files-empty">此目录暂无文件</div>
          <div v-else class="file-list">
            <div v-for="entry in entries" :key="entry.path" class="file-row" @dblclick="entry.is_dir && openDir(entry.path)">
              <t-icon :name="entry.is_dir ? 'folder' : 'file'" />
              <button class="file-name" @click="entry.is_dir ? openDir(entry.path) : undefined">{{ entry.name }}</button>
              <span class="file-size">{{ entry.is_dir ? '—' : formatSize(entry.size) }}</span>
              <template v-if="!entry.is_dir">
                <t-button size="small" variant="text" @click="download(entry)"><t-icon name="download" /></t-button>
                <t-button size="small" variant="text" @click="rename(entry)"><t-icon name="edit-1" /></t-button>
              </template>
              <t-button size="small" variant="text" theme="danger" @click="remove(entry)"><t-icon name="delete" /></t-button>
            </div>
          </div>
        </section>
      </t-tab-panel>
    </t-tabs>
  </t-drawer>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { fetchEventSource } from '@microsoft/fetch-event-source'
import { MessagePlugin } from 'tdesign-vue-next'
import { getApiBaseUrl } from '@/utils/api-base'
import {
  cancelWorkbenchCommand, deleteWorkbenchFile, downloadWorkbenchFile, listWorkbenchFiles,
  renameWorkbenchFile, startWorkbenchCommand, uploadWorkbenchFile, type WorkbenchFileEntry,
} from '@/api/chat'

const outputRoot = '/workspace/output'
const props = defineProps<{ visible: boolean; sessionId: string }>()
const emit = defineEmits<{ (e: 'update:visible', value: boolean): void }>()
const drawerVisible = computed({ get: () => props.visible, set: v => emit('update:visible', v) })
const tab = ref('terminal')
const command = ref('')
const output = ref('')
const running = ref(false)
const terminalStatus = ref('就绪')
const activeCommandId = ref('')
const streamAbort = ref<AbortController | null>(null)
const outputBox = ref<HTMLElement | null>(null)
const terminalBox = ref<HTMLElement | null>(null)
const currentPath = ref(outputRoot)
const entries = ref<WorkbenchFileEntry[]>([])
const loadingFiles = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

watch([() => props.visible, tab], ([visible, selected]) => {
  if (visible && selected === 'files') void loadFiles()
})

async function run() {
  const value = command.value.trim()
  if (!value || !props.sessionId || running.value) return
  output.value += `${output.value ? '\n' : ''}$ ${value}\n`
  command.value = ''
  running.value = true
  terminalStatus.value = '运行中'
  try {
    const response: any = await startWorkbenchCommand(props.sessionId, value)
    const data = response?.data || response
    activeCommandId.value = data.id
    await stream(data.id)
  } catch (error: any) {
    output.value += `\n[error] ${error?.message || '命令启动失败'}\n`
    terminalStatus.value = '失败'
    running.value = false
  }
}

async function stream(commandId: string) {
  const controller = new AbortController()
  streamAbort.value = controller
  const token = localStorage.getItem('weknora_token') || ''
  const tenant = localStorage.getItem('weknora_selected_tenant_id') || ''
  const url = `${getApiBaseUrl()}/api/v1/sessions/${props.sessionId}/workbench/commands/${commandId}/events`
  await fetchEventSource(url, {
    method: 'GET', signal: controller.signal, openWhenHidden: true,
    headers: { Authorization: `Bearer ${token}`, ...(tenant ? { 'X-Tenant-ID': tenant } : {}) },
    onmessage(message) {
      const event = JSON.parse(message.data)
      if (event.type === 'output') output.value += event.data || ''
      if (event.type === 'complete') {
        terminalStatus.value = event.status || '完成'
        output.value += `\n[${event.status}; exit ${event.exit_code ?? 0}]\n`
        running.value = false
        activeCommandId.value = ''
      }
      void nextTick(() => { if (outputBox.value) outputBox.value.scrollTop = outputBox.value.scrollHeight })
    },
    onclose() { running.value = false },
    onerror(error) { running.value = false; throw error },
  })
}

async function interrupt() {
  if (!activeCommandId.value) return
  await cancelWorkbenchCommand(props.sessionId, activeCommandId.value)
  terminalStatus.value = '正在中断'
}

async function loadFiles() {
  if (!props.sessionId) return
  loadingFiles.value = true
  try {
    const response: any = await listWorkbenchFiles(props.sessionId, currentPath.value)
    const data = response?.data || response
    entries.value = data.entries || []
  } catch { entries.value = []; MessagePlugin.error('无法读取沙箱文件') }
  finally { loadingFiles.value = false }
}

function openDir(filePath: string) { currentPath.value = filePath; void loadFiles() }
function goUp() { if (currentPath.value !== outputRoot) openDir(currentPath.value.slice(0, currentPath.value.lastIndexOf('/')) || outputRoot) }
async function onUpload(event: Event) {
  const input = event.target as HTMLInputElement; const file = input.files?.[0]; if (!file) return
  try { await uploadWorkbenchFile(props.sessionId, currentPath.value, file); MessagePlugin.success('上传成功'); await loadFiles() }
  catch { MessagePlugin.error('上传失败') } finally { input.value = '' }
}
async function download(entry: WorkbenchFileEntry) {
  const blob = await downloadWorkbenchFile(props.sessionId, entry.path); const url = URL.createObjectURL(blob)
  const a = document.createElement('a'); a.href = url; a.download = entry.name; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
}
async function rename(entry: WorkbenchFileEntry) {
  const value = window.prompt('新文件名', entry.name)?.trim(); if (!value || value === entry.name) return
  try { await renameWorkbenchFile(props.sessionId, entry.path, value); await loadFiles() } catch { MessagePlugin.error('重命名失败') }
}
async function remove(entry: WorkbenchFileEntry) {
  if (!window.confirm(`删除 ${entry.name}？`)) return
  try { await deleteWorkbenchFile(props.sessionId, entry.path); await loadFiles() } catch { MessagePlugin.error('删除失败') }
}
function formatSize(bytes: number) { return bytes < 1024 ? `${bytes} B` : bytes < 1048576 ? `${(bytes / 1024).toFixed(1)} KB` : `${(bytes / 1048576).toFixed(1)} MB` }
onBeforeUnmount(() => streamAbort.value?.abort())
</script>

<style scoped lang="less">
.wb-title,.terminal-toolbar,.terminal-input,.files-toolbar,.file-row{display:flex;align-items:center;gap:10px}.wb-tabs{height:100%}.terminal{height:calc(100vh - 150px);min-height:420px;background:#111827;color:#d1fae5;border-radius:10px;display:flex;flex-direction:column;overflow:hidden}.terminal-toolbar{padding:9px 12px;background:#1f2937;justify-content:space-between}.terminal-output{flex:1;margin:0;padding:14px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;font:13px/1.55 ui-monospace,SFMono-Regular,Consolas,monospace}.terminal-muted{color:#6b7280}.terminal-input{padding:10px 12px;border-top:1px solid #374151}.terminal-prompt{color:#34d399}.terminal-input input{flex:1;border:0;outline:0;background:transparent;color:#fff;font:14px ui-monospace,monospace}.files-toolbar{padding:8px 0 14px}.files-spacer{flex:1}.path-up{border:0;background:transparent;cursor:pointer}.path-up:disabled{opacity:.35}.files-empty{padding:60px;text-align:center;color:var(--td-text-color-placeholder)}.file-row{padding:9px;border-bottom:1px solid var(--td-component-stroke)}.file-name{flex:1;text-align:left;border:0;background:transparent;cursor:pointer;color:var(--td-text-color-primary)}.file-size{width:90px;text-align:right;color:var(--td-text-color-secondary);font-size:12px}
</style>
