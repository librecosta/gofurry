import * as stylex from '@stylexjs/stylex'

export const tokens = stylex.defineVars({
  surface: { default: '#fafafa', '@media (prefers-color-scheme: dark)': '#181818' },
  text: { default: '#222222', '@media (prefers-color-scheme: dark)': '#eeeeee' },
  border: { default: '#767676', '@media (prefers-color-scheme: dark)': '#aaaaaa' },
})
