import "../styles/Workspace.css";
import "../styles/Components.css";

const privateSupportConfig = {
  qq: import.meta.env.VITE_PRIVATE_SUPPORT_QQ?.trim() ?? "",
  wechat: import.meta.env.VITE_PRIVATE_SUPPORT_WECHAT?.trim() ?? "",
};

export function PrivateSupportCard() {
  const hasQQ = privateSupportConfig.qq.length > 0;
  const hasWechat = privateSupportConfig.wechat.length > 0;

  return (
    <section className="private-support panel glass-panel animate-fade-in" aria-label="专属服务">
      <div>
        <p className="eyebrow">专属服务</p>
        <h3>联系获取更多帮助</h3>
        <p className="usage-note">额度咨询、生成失败处理、低价AI会员独享账号购买请直接与下方联系。</p>
      </div>
      <dl className="support-list">
        <div>
          <dt>QQ</dt>
          <dd>{hasQQ ? privateSupportConfig.qq : "待配置"}</dd>
        </div>
        <div>
          <dt>微信</dt>
          <dd>{hasWechat ? privateSupportConfig.wechat : "待配置"}</dd>
        </div>
      </dl>
    </section>
  );
}
