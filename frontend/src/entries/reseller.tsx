import { createRoot } from 'react-dom/client';
import { message } from 'antd';
import 'antd/dist/reset.css';

import { ThemeProvider } from '@/hooks/useTheme';
import { QueryProvider } from '@/api/QueryProvider';
import ResellerPortal from '@/pages/reseller/ResellerPortal';

const messageContainer = document.getElementById('message');
if (messageContainer) {
  message.config({ getContainer: () => messageContainer });
}

const root = document.getElementById('app');
if (root) {
  createRoot(root).render(
    <ThemeProvider>
      <QueryProvider>
        <ResellerPortal />
      </QueryProvider>
    </ThemeProvider>,
  );
}
