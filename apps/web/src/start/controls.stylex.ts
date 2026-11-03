import * as stylex from '@stylexjs/stylex'
import { tokens } from '../styles/tokens.stylex'

// Local Start controls only. One inset focus outline, no native/extra ring.
export const controls = stylex.create({
  glass: {
    borderWidth: 0, borderRadius: tokens.radiusSmall, fontFamily: 'inherit',
    // Exclude keyboard focus from pointer rules: their compound selector has
    // higher specificity than :focus-visible. :not(:disabled) covers links too.
    backgroundColor: { default: tokens.overlaySurface, ':hover:not(:disabled):not(:focus-visible)': tokens.overlaySurfaceHover, ':active:not(:disabled):not(:focus-visible)': tokens.overlaySelectedSurface, ':focus-visible': tokens.overlaySelectedSurface },
    color: { default: tokens.overlayText, ':active:not(:disabled):not(:focus-visible)': tokens.overlaySelectedText, ':focus-visible': tokens.overlaySelectedText },
    backdropFilter: 'blur(14px) saturate(1.12)', boxShadow: tokens.overlayHighlight,
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    opacity: { default: 1, ':disabled': 0.55 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' },
    outlineWidth: 2, outlineOffset: -2, outlineColor: tokens.accent,
    transitionProperty: 'background-color, color', transitionDuration: tokens.motionFast,
  },
  selected: {
    backgroundColor: { default: tokens.overlaySelectedSurface, ':hover:not(:disabled):not(:focus-visible)': tokens.overlaySelectedSurface, ':active:not(:disabled):not(:focus-visible)': tokens.overlaySurfaceHover },
    color: { default: tokens.overlaySelectedText, ':active:not(:disabled):not(:focus-visible)': tokens.overlayText },
  },
})
