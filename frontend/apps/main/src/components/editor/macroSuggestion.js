import { VueRenderer } from '@tiptap/vue-3'
import MacroSuggestionList from './MacroSuggestionList.vue'

// Fixed popup dimensions — must match those in MacroSuggestionList.vue
const REM = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
const POPUP_WIDTH = 40 * REM       // 40rem — must match MacroSuggestionList
const POPUP_HEIGHT = 18.7 * REM   // 18.75rem — must match MacroSuggestionList
const VIEWPORT_MARGIN = 0.5 * REM
const CURSOR_GAP = 0.25 * REM

export default {
  char: '/',
  allowSpaces: false,

  allow: ({ editor }) => editor.options.editorProps?.messageType !== 'forward',

  items: async ({ query, editor }) => {
    const getMacros = editor.options.editorProps?.getMacros
    if (!getMacros) return []

    return await getMacros(query)
  },

  command: ({ editor, range, props: macro }) => {
    editor.chain().focus().deleteRange(range).run()

    if (macro?.message_content) {
      editor.chain().focus().insertContent(macro.message_content).run()
    }

    editor.options.editorProps?.onMacroPicked?.(macro)
  },

  render: () => {
    let component
    let popup
    let editor

    return {
      onStart: (props) => {
        editor = props.editor
        component = new VueRenderer(MacroSuggestionList, {
          props: {
            ...props,
            query: props.query
          },
          editor: props.editor
        })

        if (!props.clientRect) {
          return
        }

        popup = document.createElement('div')
        popup.style.position = 'fixed'
        popup.style.zIndex = '9999'
        popup.addEventListener(
          'pointerdown',
          () => {
            editor?.options.editorProps?.onSuppressOutsideCollapse?.()
          },
          { capture: true }
        )
        if (component.element) {
          popup.appendChild(component.element)
        }
        document.body.appendChild(popup)
        editor?.options.editorProps?.onComposerOverlayChange?.('macro-suggestion', true)

        positionPopup(popup, props.clientRect)
      },

      onUpdate: (props) => {
        component.updateProps({
          ...props,
          query: props.query
        })
        // Intentionally do NOT reposition on update. The popup is anchored
        // when the trigger first fires; navigating between macros (or
        // filtering as the user types more characters) keeps the popup put.
        // This is what Intercom does and avoids the popup hopping around.
      },

      onKeyDown: (props) => {
        if (props.event.key === 'Escape') {
          if (popup) {
            popup.style.display = 'none'
          }
          return component.ref?.onKeyDown(props) || true
        }

        return component.ref?.onKeyDown(props)
      },

      onExit: () => {
        editor?.options.editorProps?.onComposerOverlayChange?.('macro-suggestion', false)
        if (popup && popup.parentNode) {
          popup.parentNode.removeChild(popup)
        }
        component.destroy()
      }
    }
  }
}

// Decide once where the popup sits, based on cursor location and fixed popup
// dimensions. The popup never resizes after this, so the placement is stable
// even as the user navigates between macros of different content lengths.
function positionPopup(popup, clientRect) {
  const rect = clientRect()
  if (!rect) return

  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight

  // Horizontal: anchor to cursor, clamp to viewport.
  let left = rect.left
  if (left + POPUP_WIDTH > viewportWidth - VIEWPORT_MARGIN) {
    left = Math.max(VIEWPORT_MARGIN, viewportWidth - POPUP_WIDTH - VIEWPORT_MARGIN)
  } else if (left < VIEWPORT_MARGIN) {
    left = VIEWPORT_MARGIN
  }

  // Vertical: prefer below cursor; flip above if it would overflow; clamp
  // to top of viewport if neither side has room for the full popup.
  const spaceBelow = viewportHeight - rect.bottom - CURSOR_GAP - VIEWPORT_MARGIN
  const spaceAbove = rect.top - CURSOR_GAP - VIEWPORT_MARGIN

  let top
  if (POPUP_HEIGHT <= spaceBelow) {
    top = rect.bottom + CURSOR_GAP
  } else if (POPUP_HEIGHT <= spaceAbove) {
    top = rect.top - POPUP_HEIGHT - CURSOR_GAP
  } else {
    // Neither side fits the popup at full size. Pin to the top of the
    // viewport — the popup's overflow handling keeps its content scrollable.
    top = VIEWPORT_MARGIN
  }

  popup.style.left = `${left}px`
  popup.style.top = `${top}px`
}
