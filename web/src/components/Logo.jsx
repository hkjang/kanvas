import React from 'react'

export function Logo({ small = false }) {
  return <span className={`logo-mark ${small ? 'small' : ''}`} aria-label="Kanvas"><i>K</i></span>
}
