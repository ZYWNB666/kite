export function isSidebarRouteActive(pathname: string, url: string) {
  if (url === '/' || url === '/crds') {
    return pathname === url
  }

  return pathname === url || pathname.startsWith(`${url}/`)
}
