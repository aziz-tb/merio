export function useFileDropAttachments (onFilesDropped) {
    const handleDrop = (view, event) => {
        const droppedFiles = event.dataTransfer?.files
        if (!droppedFiles || droppedFiles.length === 0) return false

        event.preventDefault()
        event.stopPropagation()

        const files = Array.from(droppedFiles).filter((file) => file && file.size > 0)
        if (files.length === 0) return true

        if (typeof onFilesDropped === 'function') {
            onFilesDropped(files)
        }

        return true
    }

    return { handleDrop }
}
