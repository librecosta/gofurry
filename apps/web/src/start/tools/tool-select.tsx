import { useRef, useState, type ReactNode } from 'react'
import { Select } from '@base-ui/react/select'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr/CaretDown'
import { CheckIcon } from '@phosphor-icons/react/dist/ssr/Check'
import { AnimatePresence, motion, useIsPresent } from 'motion/react'
import { useMotionTiming } from '../../motion'
import { cx } from '../../styles/classes'
import chrome from '../chrome.module.scss'
import controls from '../controls.module.scss'
import styles from './tools.module.scss'

export function ToolSelect<Value extends string>({ label, items, value, onChange }: {
  label: string; items: { value: Value; label: string }[]; value: Value; onChange: (value: Value) => void
}) {
  const [open, setOpen] = useState(false)
  const actions = useRef<Select.Root.Actions>(null)

  return <div className={styles.field}>
    <Select.Root<Value> value={value} items={items} modal={false} open={open} actionsRef={actions}
      onValueChange={next => { if (next !== null) onChange(next) }}
      onOpenChange={(next, details) => { if (!next) details.preventUnmountOnClose(); setOpen(next) }}>
      <Select.Label>{label}</Select.Label>
      <Select.Trigger className={cx(controls.glass, styles.selectTrigger)}>
        <Select.Value className={styles.selectValue} />
        <Select.Icon className={styles.selectIcon}><CaretDownIcon size={13} aria-hidden="true" /></Select.Icon>
      </Select.Trigger>
      <Select.Portal>
        <AnimatePresence onExitComplete={() => { if (!open) actions.current?.unmount() }}>
          {open && <Options>
            {items.map(item => <Select.Item value={item.value} key={item.value} className={styles.selectOption}>
              <Select.ItemIndicator><CheckIcon size={13} weight="bold" aria-hidden="true" /></Select.ItemIndicator>
              <Select.ItemText>{item.label}</Select.ItemText>
            </Select.Item>)}
          </Options>}
        </AnimatePresence>
      </Select.Portal>
    </Select.Root>
  </div>
}

function Options({ children }: { children: ReactNode }) {
  const transition = useMotionTiming(0.18)
  const present = useIsPresent()
  return <Select.Positioner className={styles.selectPositioner} side="bottom" align="start" sideOffset={4}
    positionMethod="fixed" collisionPadding={8} alignItemWithTrigger={false} inert={!present} aria-hidden={!present || undefined}>
    <Select.Popup className={cx(chrome.menu, styles.selectPopup)}
      render={<motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition} />}>
      <Select.List>{children}</Select.List>
    </Select.Popup>
  </Select.Positioner>
}
