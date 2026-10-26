import type { PropsWithChildren } from 'react';
import AppRoot from './platform/app-root';
export default function App({ children }: PropsWithChildren) {
  return <AppRoot>{children}</AppRoot>;
}
