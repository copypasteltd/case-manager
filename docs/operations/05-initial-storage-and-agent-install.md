# 首次存储配置与秘书运行配置安装

这两个命令只在部署维护环境中运行，需要独立`CP_MIGRATION_DATABASE_URL`及正常134–136升级，不能把迁移连接交给API。生产PG必须验证服务器证书与主机名。输入为绝对路径的普通私有JSON文件，Linux权限0600；不允许符号链接、未知字段、第二份JSON或超过64 KiB输入。操作回执单独追加保存，应用身份无写权限；输出只有回执编号，不回显路径、配置和连接错误。

## 初始存储

设置与API配置一致的`CP_STORAGE_DRIVER`、`CP_OSS_BUCKET`、`CP_R2_BUCKET`。在未配置quota／epoch／targets且尚无文件／上传的组织内，准备：

```json
{"organization_id":"<真实组织UUID>","actor_id":"<当前有效经办人UUID>","reason":"已核对首次存储配置","primary":"oss","oss_bucket":"<准确OSS桶>","r2_bucket":"<准确R2桶>","logical_limit_bytes":"<明确批准的字节数>","reviewed":true}
```

```text
/opt/copypaste/releases/<已核对版本>/bin/platform provision-storage --file /etc/copypaste/private/initial-storage.json
```

三个元数据表和回执同一事务提交。相同固定配置可重跑核对回执；不能用这个入口调整既有配额、修改主端或修补手工部分配置，遇到既有状态即停止。初始epoch的依据明确为“空存储首次配置”，不声称已做云切换围栏或云端可用性验证。

对象服务账号、最小权限、私有桶、原生版本／独占不可变管理边界、CORS、平台request合法域名、扫描和各独立Worker配置仍依照现有存储／部署指南准备；此命令不调用云开户或凭据API，不替代真实接入验收。

## 安装秘书运行配置

```text
/opt/copypaste/releases/<已核对版本>/bin/platform install-agent-profile --file /etc/copypaste/private/agent-install.json
```

输入字段定义见应用`internal/deployment.AgentInput`。准备以下真实资料后才执行：

1. `organization_id`、有效`actor_id`、200字以内`reason`和`reviewed:true`。
2. 原生`node_path`绝对路径；已核对只读发布目录中`describe_script:{path,sha256}`，对应打包后的`app/agent-runtime/dist/describe-profile.js`。从可信发布清单核对摘要，不能使用普通账号可修改的代码目录。
3. `inspection:{skill_id,profile_id,model,working_directory,mcp_endpoint,api_base_url?}`。参数与原生broker及网关部署完全一致；检查器只描述静态配置，不调用模型或修改数据库。
4. 提前核对的`expected_skill_sha256`、`expected_configuration_sha256`，以及明确的技术输入／输出限制`max_input_tokens`、`max_output_tokens`；工具数和时限从实际包描述取得。这些是技术资源边界，不增加AI费用配额。
5. `evaluation:{set_version,set_sha256,results_sha256,reference,passed,failed,skipped}`；`dataset`、`results`分别为`{path,sha256}`，必须与评估摘要相同。结果数量须为真实外部评定，不能把当前合成测试填进去冒充验收。
6. `native_isolation`、`model_contract`各提供`{path,sha256}`，固定真实原生隔离及模型契约的已批准前置证据。安装器核对文件摘要并保存“部署经办人声明的外部证据”身份，不自动重演／证明文件里的实际结果。

静态描述器必须返回准确published包、全部工具可用及准确SDK版本。安装器隔离环境变量，不向描述进程传递提供商、数据库或任务秘密；立即重新描述及核对前置文件后，再事务写入配置、原有安装事件及追加部署回执。配置ID不可覆盖，同一Skill已有active配置时不会自动替换／停用；已有停用配置不会重新激活。相同输入重跑仅核对原安装回执，不保证该配置现在仍active。

目前仓库八包均draft，真实安装预期拒绝，直到公司独立评估、批准及完整发布产物准备完成。不得手改state绕过这一步。之后人工发布台账仍另需最近MFA、本人准确配置成功运行及版本化外部评估资料；运行成功和外部评定分别保存。
