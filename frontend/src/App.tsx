import Router from './router';
import ErrorBoundary from './components/ErrorBoundary';
import { I18nProvider } from './contexts/I18nContext';

function App() {
  return (
    <ErrorBoundary>
      <I18nProvider>
        <Router />
      </I18nProvider>
    </ErrorBoundary>
  );
}

export default App;
