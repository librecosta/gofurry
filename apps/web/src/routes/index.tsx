import { createFileRoute, Link } from '@tanstack/react-router'
import { Popover } from '@base-ui/react/popover'
import * as stylex from '@stylexjs/stylex'
import { tokens } from '../styles/tokens.stylex'

export const Route = createFileRoute('/')({ component: RuntimeCheck })

function RuntimeCheck() {
  return (
    <main {...stylex.props(styles.page)}>
      <h1>GoFurry runtime skeleton</h1>
      <p>V0.1-A · SSR / StyleX / Base UI</p>
      <Popover.Root>
        <Popover.Trigger {...stylex.props(styles.button)}>验证交互</Popover.Trigger>
        <Popover.Portal>
          <Popover.Positioner sideOffset={8}>
            <Popover.Popup {...stylex.props(styles.popup)}>
              <Popover.Title>基础交互检查</Popover.Title>
              <Popover.Description>支持键盘、焦点与系统明暗主题。</Popover.Description>
              <Popover.Close {...stylex.props(styles.button)}>关闭</Popover.Close>
            </Popover.Popup>
          </Popover.Positioner>
        </Popover.Portal>
      </Popover.Root>
      {' '}
      <Popover.Root>
        <Popover.Trigger disabled {...stylex.props(styles.button)}>禁用状态</Popover.Trigger>
      </Popover.Root>
      <p><Link to="/status">运行状态</Link></p>
    </main>
  )
}

const styles = stylex.create({
  page: { minHeight: '100vh', padding: 32, backgroundColor: tokens.surface, color: tokens.text },
  button: {
    paddingBlock: 8, paddingInline: 12, borderRadius: 4,
    borderWidth: 1, borderStyle: 'solid', borderColor: tokens.border,
    backgroundColor: tokens.surface, color: tokens.text,
    opacity: { default: 1, ':disabled': 0.5 },
    outlineOffset: 3,
  },
  popup: {
    padding: 20, maxWidth: 320, borderRadius: 4,
    borderWidth: 1, borderStyle: 'solid', borderColor: tokens.border,
    backgroundColor: tokens.surface, color: tokens.text,
  },
})
