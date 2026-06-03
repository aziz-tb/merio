import { FileLines as FileText, FileSpreadsheet, FileImage, FileVideo, FileAudio, FileZipper as FileArchive, File } from '@qall/icons'

const ICON_MAP = {
  // Documents
  pdf: FileText,
  doc: FileText, docx: FileText, txt: FileText, rtf: FileText, odt: FileText,
  // Spreadsheets
  xls: FileSpreadsheet, xlsx: FileSpreadsheet, csv: FileSpreadsheet, ods: FileSpreadsheet,
  // Images
  png: FileImage, jpg: FileImage, jpeg: FileImage, gif: FileImage,
  webp: FileImage, svg: FileImage, bmp: FileImage,
  // Video
  mp4: FileVideo, mov: FileVideo, avi: FileVideo, webm: FileVideo, mkv: FileVideo,
  // Audio
  mp3: FileAudio, wav: FileAudio, ogg: FileAudio, m4a: FileAudio, flac: FileAudio,
  // Archives
  zip: FileArchive, rar: FileArchive, '7z': FileArchive,
  tar: FileArchive, gz: FileArchive,
}

/**
 * Returns the appropriate lucide icon component for a filename.
 * @param {string} filename - The filename (with or without path)
 * @returns Vue component for the icon
 */
export function getFileIcon(filename) {
  const ext = (filename || '').toLowerCase().split('.').pop()
  return ICON_MAP[ext] || File
}

/**
 * Returns true if the filename has an image extension.
 */
export function isImageFile(filename) {
  const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp']
  const ext = (filename || '').toLowerCase().split('.').pop()
  return imageExts.includes(ext)
}