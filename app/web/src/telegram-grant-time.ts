// A ten-minute grant must expose its actual Hong Kong time, including seconds.
export const grantTime=(value:string)=>new Intl.DateTimeFormat('zh-HK',{timeZone:'Asia/Hong_Kong',year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).format(new Date(value))+' · 香港时间';
