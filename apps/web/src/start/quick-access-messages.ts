import { usePreferences } from '../preferences'

const messages = {
  zh: {
    toggle: '显示快捷访问', title: '管理快捷访问', close: '关闭快捷访问管理', pinned: '常驻站点', recent: '最近访问',
    add: '添加网站', edit: '编辑', name: '名称', url: '网址', save: '保存', cancel: '取消编辑', empty: '尚无常驻站点', noRecent: '尚无最近访问',
    invalid: '请填写名称和有效 HTTP/HTTPS 网址，不可包含账号密码、重复站点；最多保存 128 个常驻站点。',
    open: '打开', pin: '固定到快捷访问', unpin: '取消固定', remove: '删除', removeRecent: '从最近访问中移除',
    unpinHint: '保留已有最近访问记录', deleteHint: '同时移除常驻和最近访问记录', actions: '站点操作', up: '上移', down: '下移', drag: '拖动排序',
  },
  en: {
    toggle: 'Show quick access', title: 'Manage quick access', close: 'Close quick access manager', pinned: 'Pinned sites', recent: 'Recent visits',
    add: 'Add site', edit: 'Edit', name: 'Name', url: 'URL', save: 'Save', cancel: 'Cancel edit', empty: 'No pinned sites yet', noRecent: 'No recent visits yet',
    invalid: 'Enter a name and valid HTTP/HTTPS URL without credentials or duplicate sites. Up to 128 pinned sites are supported.',
    open: 'Open', pin: 'Pin to quick access', unpin: 'Unpin', remove: 'Delete', removeRecent: 'Remove from recent visits',
    unpinHint: 'Keep existing recent visits', deleteHint: 'Remove both pin and recent visits', actions: 'Site actions', up: 'Move up', down: 'Move down', drag: 'Drag to reorder',
  },
}
export function useQuickMessages() { return messages[usePreferences().locale] }
