export function Nav() {
  return (
    <header className="db-nav">
      <div className="db-nav-left">
        <div className="db-brand">
          <span className="db-brand-mark" />
          <span>Rive</span>
          <span className="db-brand-tag">Dashboard</span>
        </div>
        <nav className="db-nav-links">
          <a href="#" className="active">
            Agent P&amp;L
          </a>
          <a href="#">Work orders</a>
          <a href="#">Netting</a>
          <a href="#">Docs</a>
        </nav>
      </div>
      <div className="db-nav-right">
        <span className="db-network-badge">0G Mainnet</span>
{/* <button className="db-btn db-btn-solid">Connect wallet</button> */}
      </div>
    </header>
  );
}
