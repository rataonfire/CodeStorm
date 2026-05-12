import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';

interface DiscrepancyChartProps {
  data: { time: string; discrepancies: number }[];
}

export function DiscrepancyChart({ data }: DiscrepancyChartProps) {
  if (data.length === 0) {
    return <div className="text-sm text-gray-400 p-4">Нет данных для графика</div>;
  }

  return (
    <div className="bg-white/80 backdrop-blur-sm p-5 rounded-2xl shadow-lg border border-primary-100">
      <h3 className="text-lg font-semibold text-dark-800 mb-4">
        Расхождения в минуту (за последний час)
      </h3>
      <ResponsiveContainer width="100%" height={300}>
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" stroke="e9d5ff" />
          <XAxis dataKey="time" tick={{ fontSize: 12, fill: '#3b1f6e' }} interval="preserveStartEnd" />
          <YAxis allowDecimals={false} tick={{ fill: '#3b1f6e' }} />
          <Tooltip contentStyle={{ backgroundColor: '#0f2440', border: 'none', borderRadius: '12px', color: '#fff' }}/>
          <Line
            type="monotone"
            dataKey="discrepancies"
            stroke="#ef4444"
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 6, fill: '#845ef7' }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}