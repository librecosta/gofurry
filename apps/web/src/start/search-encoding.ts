// Faloo's official Common.js uses legacy escape(), not UTF-8 URL encoding.
// Encode UTF-16 code units explicitly without depending on deprecated globals.
export function encodeFalooKeyword(keyword: string) {
  return keyword.replace(/[^A-Za-z0-9@*_+./-]/g, char => {
    const code = char.charCodeAt(0)
    return code < 256 ? `%${code.toString(16).toUpperCase().padStart(2, '0')}` : `%u${code.toString(16).toUpperCase().padStart(4, '0')}`
  })
}
