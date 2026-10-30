# Office 空白模板来源 / Blank OOXML templates

三个空白模板通过 `app/scripts/build_office_templates.py` 生成，随 Go 二进制嵌入。生成环境为现有原生 Python，使用 python-docx1.2.0、openpyxl3.1.5、python-pptx1.0.2 的默认空白文档；均为MIT，完整原许可证随本目录保存。生成时固定文档作者与核心时间、去除预览图、ZIP排序／时间／权限固定。无字体、Office可执行程序、外部链接、宏或公司业务记录。运行时无需这三个 Python 库。

| 模板 | 字节 | SHA-256 |
|---|---:|---|
| `empty.docx` | 34934 | `b5feb28d01af57dca6b3231ffa01ce9eeb270698a6caa31ab28fddccd97b8a28` |
| `empty.xlsx` | 4962 | `d1cdd1609c4239180910e91fdcd1a188f178b3eaab3692ef634ecd23a63ce53d` |
| `empty.pptx` | 25695 | `62418bdea918081c657092997a1a1f9d79bccb4a566fed796ddebbd13ae40aeb` |

两次独立生成得到同一摘要，清单见 [manifest](../../../app/backend/internal/files/office_templates/manifest.json)。修改模板或生成器需要新的源码核验和真实文件消费检查；不能覆盖历史证据。

上游：[python-docx](https://github.com/python-openxml/python-docx)、[openpyxl](https://openpyxl.readthedocs.io/en/stable/)、[python-pptx](https://github.com/scanny/python-pptx)。[DOCX许可](python-docx-LICENSE.txt)、[XLSX许可](openpyxl-LICENCE.rst.txt)、[PPTX许可](python-pptx-LICENSE.txt)。这些源代码库不提供 Microsoft Office 视觉或全部特性兼容保证。

The embedded blank templates come from the default documents shipped with the three pinned MIT libraries above. ZIP metadata and core document metadata are normalized; no preview image, font, executable, macro, external link or company data is included. Two independent generation runs produced identical hashes. The deployed Go authoring service does not execute Python or download templates. Preserve upstream notices and reverify actual outputs whenever these inputs change; production artifact acceptance remains separate.
