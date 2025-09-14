const express = require('express');
const app = express();
const PORT = 3001;

app.use(express.json());

app.get('/api/v1/ping', (req, res) => {
  res.json({ message: 'pong' });
});

app.listen(PORT, () => {
  console.log(`Server running on http://localhost:${PORT}`);
});
