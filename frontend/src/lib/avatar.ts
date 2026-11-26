// Фото демо-аккаунтов: файлы в public/avatars/<id персоны>.png (self-host). Нет фото — показываются инициалы.
const WITH_PHOTO = new Set([
  'buyer-smb',
  'buyer-enterprise',
  'buyer-gov',
  'staff-ecom',
  'staff-product',
  'staff-content',
  'staff-head',
])

export function personaPhoto(personaId: string | null | undefined): string | null {
  return personaId && WITH_PHOTO.has(personaId) ? `/avatars/${personaId}.png` : null
}
