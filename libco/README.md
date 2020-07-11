##libco是微信后台大规模使用的c/c++协程库，2013年至今稳定运行在微信后台的数万台机器上。
作者: sunnyxu(sunnyxu@tencent.com), leiffyli(leiffyli@tencent.com)

libco通过仅有的几个函数接口 co_create/co_resume/co_yield 再配合 co_poll，可以支持同步或者异步的写法，如线程库一样轻松。同时库里面提供了socket族函数的hook，使得后台逻辑服务几乎不用修改逻辑代码就可以完成异步化改造。

同时提供协程变量机制辅助代码快速修改，并提供env族函数的hook以支持cgi轻松切换到协程模式，而新的共享栈模式（可选）轻松构建单机千万连接

原理 https://blog.csdn.net/u010318270/article/details/94044361?utm_medium=distribute.pc_relevant_right.none-task-blog-BlogCommendFromMachineLearnPai2-5.nonecase&depth_1-utm_source=distribute.pc_relevant_right.none-task-blog-BlogCommendFromMachineLearnPai2-5.nonecase
