import { TransactionTable } from './components/TransactionTable';
import { DiscrepancyChart } from './components/DiscrepancyChart';
import { useWebSocket } from './hooks/useWebSocket';

function App() {
  const { history } = useWebSocket();

  return (
    <div className="min-h-screen bg-gradient-to-br from-primary-50 to-dark-900 p-6 font-sans">
      <header className="mb-8">
        <h1 className="text-3xl font-bold text-dark-800 tracking-tight">
          Панель оператора — Сверка транзакций
        </h1>
        <p className="text-sm text-primary-700 mt-1">
          Состояние источников A, B, C в реальном времени
        </p>
      </header>

      <main className="space-y-8">
        <TransactionTable />
        <DiscrepancyChart data={history} />
      </main>
    </div>
  );
}

export default App;