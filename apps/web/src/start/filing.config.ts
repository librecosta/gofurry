// Public, explicit deployment metadata. Never copy another domain's numbers.
// Populate only after approval for this site, using the official issued icon/URL.
// Pending labels are for local review; set showPending=false before publication.
type FilingConfig = {
  showPending: boolean
  icp: { number: string } | null
  publicSecurity: { number: string; icon: `/${string}`; queryUrl: `https://beian.mps.gov.cn/${string}` } | null
}

export const cnFiling: FilingConfig = {
  showPending: true,
  icp: null,
  publicSecurity: null,
}
